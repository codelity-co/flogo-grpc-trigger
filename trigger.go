package grpc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/project-flogo/core/data"
	"github.com/project-flogo/core/data/coerce"
	"github.com/project-flogo/core/data/mapper"
	"github.com/project-flogo/core/data/metadata"
	"github.com/project-flogo/core/data/property"
	"github.com/project-flogo/core/data/resolve"
	"github.com/project-flogo/core/support/log"
	"github.com/project-flogo/core/trigger"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
)

var triggerMd = trigger.NewMetadata(&Settings{}, &HandlerSettings{}, &Output{})
var resolver = resolve.NewCompositeResolver(map[string]resolve.Resolver{
	".":        &resolve.ScopeResolver{},
	"env":      &resolve.EnvResolver{},
	"property": &property.Resolver{},
	"loop":     &resolve.LoopResolver{},
})

func init() {
	_ = trigger.Register(&Trigger{}, &Factory{})
}

// Factory is a gRPC Trigger factory
type Factory struct {
	metadata *trigger.Metadata
}

// New creates a new trigger instance
func (*Factory) New(config *trigger.Config) (trigger.Trigger, error) {
	s := &Settings{}
	err := metadata.MapToStruct(config.Settings, s, true)
	if err != nil {
		return nil, err
	}
	return &Trigger{settings: s, config: config}, nil
}

// Metadata get trigger metadata
func (f *Factory) Metadata() *trigger.Metadata {
	return triggerMd
}

// Handler struct
type Handler struct {
	handler  trigger.Handler
	settings *HandlerSettings
}

// Trigger is a stub for your gRPC Trigger implementation
type Trigger struct {
	config            *trigger.Config
	settings          *Settings
	handlers          map[string]*Handler
	defaultHandler    *Handler
	grpcServer        *grpc.Server
	Logger            log.Logger
	contextCancelFunc context.CancelFunc
}

// Metadata implements trigger.Trigger.Metadata
func (t *Trigger) Metadata() *trigger.Metadata {
	return triggerMd
}

// Initialize implements trigger.Trigger.Initialize
func (t *Trigger) Initialize(ctx trigger.InitContext) error {

	logger := ctx.Logger()

	s := &Settings{}

	err := s.FromMap(t.config.Settings)
	if err != nil {
		return err
	}

	logger.Debugf("Settings: %v", s)

	t.Logger = logger

	handlers := make(map[string]*Handler)
	ctxHandlers := ctx.GetHandlers()
	logger.Debugf("length of ctxHandlers: %v", len(ctxHandlers))

	for _, handler := range ctxHandlers {
		settings := &HandlerSettings{}
		err := metadata.MapToStruct(handler.Settings(), settings, true)
		if err != nil {
			return err
		}

		if settings.MethodName == "" && t.defaultHandler == nil {
			t.defaultHandler = &Handler{
				handler:  handler,
				settings: settings,
			}
		}

		handlers[settings.ServiceName+"_"+settings.MethodName] = &Handler{
			handler:  handler,
			settings: settings,
		}

	}

	t.handlers = handlers
	t.Logger.Debugf("handlers: %v", handlers)

	t.Logger.Debugf("Enable TLS: %t", t.settings.EnableTLS)
	if t.settings.EnableTLS {
		// decode server cert and server key
		serverCert, err := t.decodeCertificate(t.settings.ServerCert)
		if err != nil {
			t.Logger.Errorf("Error decoding server certificate: %s", err.Error())
			return err
		}
		serverKey, err := t.decodeCertificate(t.settings.ServerKey)
		if err != nil {
			t.Logger.Errorf("Error decoding server key: %s", err.Error())
			return err
		}
		t.settings.ServerCert = string(serverCert)
		t.settings.ServerKey = string(serverKey)
	}
	return nil
}

// Start implements trigger.Trigger.Start
func (t *Trigger) Start() error {

	// Prepare grpc server address
	grpcAddr := ":" + strconv.Itoa(t.settings.GrpcPort)

	// Prepare http server address
	var httpAddr string
	if t.settings.HttpPort > 0 {
		httpAddr = ":" + strconv.Itoa(t.settings.HttpPort)
	}

	// Create gRPC Listener
	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		t.Logger.Error(err)
		return err
	}

	var mux *runtime.ServeMux
	if t.settings.HttpPort > 0 {
		mux = runtime.NewServeMux()
	}

	// Prepare grpcListener options
	grpcOpts := []grpc.ServerOption{}

	if t.settings.EnableTLS {
		cert, err := tls.X509KeyPair([]byte(t.settings.ServerCert), []byte(t.settings.ServerKey))
		if err != nil {
			t.Logger.Error(err)
			return err
		}
		creds := credentials.NewServerTLSFromCert(&cert)
		grpcOpts = []grpc.ServerOption{grpc.Creds(creds)}
	}

	// Create gRPC server
	t.grpcServer = grpc.NewServer(grpcOpts...)

	// Regisetr grpc services
	protoName := t.settings.ProtoName
	protoName = strings.Split(protoName, ".")[0]

	t.Logger.Info(ServiceRegistery.ServerServices)
	// Register each serviceName + protoName
	if len(ServiceRegistery.ServerServices) != 0 {
		for k, service := range ServiceRegistery.ServerServices {
			servRegFlag := false
			if strings.Compare(k, protoName+service.ServiceInfo().ServiceName) == 0 {
				t.Logger.Infof("Registered Proto [%v] and Service [%v]", protoName, service.ServiceInfo().ServiceName)
				service.RunRegisterServerService(t.grpcServer, t)
				servRegFlag = true
			}
			if !servRegFlag {
				t.Logger.Errorf("Proto [%s] and Service [%s] not registered", protoName, service.ServiceInfo().ServiceName)
				return fmt.Errorf("Proto [%s] and Service [%s] not registered", protoName, service.ServiceInfo().ServiceName)
			}
			if t.settings.HttpPort > 0 {
				ctx := context.Background()
				ctx, t.contextCancelFunc = context.WithCancel(ctx)
				service.RegisterHttpMuxHandler(ctx, mux)
			}
		}

	} else {
		t.Logger.Error("gRPC server services not registered")
		return errors.New("gRPC server services not registered")
	}

	// Start grpcListener
	t.Logger.Debug("Starting server on port", grpcAddr)

	go func() {
		t.grpcServer.Serve(grpcListener)
		t.Logger.Infof("gRPC Server started on port: [%d]", t.settings.GrpcPort)
		if t.settings.EnableTLS {
			http.ListenAndServeTLS(httpAddr, t.settings.ServerCert, t.settings.ServerKey, mux)
		} else {
			http.ListenAndServe(httpAddr, mux)
		}
	}()

	return nil
}

// Stop implements trigger.Trigger.Start
func (t *Trigger) Stop() error {
	// stop the trigger
	t.grpcServer.GracefulStop()
	return nil
}

// CallHandler is to call a particular handler based on method name
func (t *Trigger) CallHandler(grpcData map[string]interface{}) (int, interface{}, error) {
	t.Logger.Debug("CallHandler method invoked")

	params := make(map[string]interface{})
	var content map[string]interface{}

	m := jsonpb.Marshaler{OrigName: true, EmitDefaults: true}
	// blocking the code for streaming requests
	if grpcData["contextData"] != nil {
		t.Logger.Debug("grpcData['contextData'] is not nil")

		// getting values from inputrequestdata and mapping it to params which can be used in different services like HTTP pathparams etc.
		s := reflect.ValueOf(grpcData["reqData"]).Elem()
		typeOfS := s.Type()
		for i := 0; i < s.NumField(); i++ {
			f := s.Field(i)
			fieldName := proto.GetProperties(typeOfS).Prop[i].OrigName
			if !strings.HasPrefix(fieldName, "XXX_") {
				// XXX_ fields will not be mapped
				if _, ok := f.Interface().(proto.Message); ok {
					jsonString, err := m.MarshalToString(f.Interface().(proto.Message))
					if err != nil {
						t.Logger.Errorf("Marshal failed on field: %s with value: %v", fieldName, f.Interface())
					}
					t.Logger.Debugf("Marshaled FieldName: [%s] Value: [%s]", fieldName, jsonString)
					var paramValue map[string]interface{}
					json.Unmarshal([]byte(jsonString), &paramValue)
					params[fieldName] = paramValue
				} else {
					t.Logger.Debugf("Field name: [%s] Value: [%v]", fieldName, f.Interface())
					params[fieldName] = f.Interface()
				}
			}
		}

		// assign req data content to trigger content
		t.Logger.Debugf("grpcData['reqData']: %v", grpcData["reqData"])

		dataBytes, err := json.Marshal(grpcData["reqData"])
		if err != nil {
			t.Logger.Error("Marshal failed on grpc request data")
			return 0, nil, err
		}

		err = json.Unmarshal(dataBytes, &content)
		if err != nil {
			t.Logger.Error("Unmarshal failed on grpc request data")
			return 0, nil, err
		}
	}

	t.Logger.Debugf("grpcData['serviceName']: %v", grpcData["serviceName"])
	t.Logger.Debugf("grpcData['methodName']: %v", grpcData["methodName"])

	handler, ok := t.handlers[grpcData["serviceName"].(string)+"_"+grpcData["methodName"].(string)]
	if !ok {
		handler = t.defaultHandler
	}

	t.Logger.Debugf("handler is nil: %v", handler == nil)

	if handler != nil {
		grpcData["protoName"] = t.settings.ProtoName

		out := &Output{
			Params:             params,
			GrpcData:           grpcData,
			ProtobufRequestMap: content,
		}

		t.Logger.Debug("Dispatch Found for ", handler.settings.ServiceName+"_"+handler.settings.MethodName)
		t.Logger.Debugf("Calling handler with params: %v", params)
		results, err := handler.handler.Handle(context.Background(), out)
		if err != nil {
			return 0, nil, err
		}
		reply := &Reply{}
		err = reply.FromMap(results)
		t.Logger.Debugf("Result from handler: %v", reply.Data)
		return reply.Code, reply.Data, err
	}

	t.Logger.Error("Dispatch not found")
	return 0, nil, errors.New("Dispatch not found")
}

func (t *Trigger) decodeCertificate(cert string) ([]byte, error) {
	if cert == "" {
		return nil, fmt.Errorf("Certificate is Empty")
	}

	// case 1: if certificate comes from fileselctor it will be base64 encoded
	if strings.HasPrefix(cert, "{") {
		t.Logger.Debug("Certificate received from file selector")
		certObj, err := coerce.ToObject(cert)
		if err == nil {
			certValue, ok := certObj["content"].(string)
			if !ok || certValue == "" {
				return nil, fmt.Errorf("No content found for certificate")
			}
			return base64.StdEncoding.DecodeString(strings.Split(certValue, ",")[1])
		}
		return nil, err
	}

	// case 2: if the certificate is defined as application property in the format "<encoding>,<encodedCertificateValue>"
	index := strings.IndexAny(cert, ",")
	if index > -1 {
		//some encoding is there
		t.Logger.Debug("Certificate received from application property with encoding")
		encoding := cert[:index]
		certValue := cert[index+1:]

		if strings.EqualFold(encoding, "base64") {
			return base64.StdEncoding.DecodeString(certValue)
		}
		return nil, fmt.Errorf("Error parsing the certificate or given encoding may not be supported")
	}

	// case 3: if the certificate is defined as application property that points to a file
	if strings.HasPrefix(cert, "file://") {
		// app property pointing to a file
		t.Logger.Debug("Certificate received from application property pointing to a file")
		fileName := cert[7:]
		return ioutil.ReadFile(fileName)
	}

	// case 4: if certificate is defined as path to a file (in oss)
	if strings.Contains(cert, "/") || strings.Contains(cert, "\\") {
		t.Logger.Debug("Certificate received from settings as file path")
		_, err := os.Stat(cert)
		if err != nil {
			t.Logger.Errorf("Cannot find certificate file: %s", err.Error())
		}
		return ioutil.ReadFile(cert)
	}

	t.Logger.Debug("Certificate received from application property without encoding")
	return []byte(cert), nil
}

func resolveObject(object map[string]interface{}) (map[string]interface{}, error) {
	var err error

	mapperFactory := mapper.NewFactory(resolver)
	valuesMapper, err := mapperFactory.NewMapper(object)
	if err != nil {
		return nil, err
	}

	objectValues, err := valuesMapper.Apply(data.NewSimpleScope(map[string]interface{}{}, nil))
	if err != nil {
		return nil, err
	}

	return objectValues, nil
}
