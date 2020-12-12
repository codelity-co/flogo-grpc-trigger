package grpc

import (
	"github.com/project-flogo/core/data/coerce"
)

type Settings struct {
	Enabled           bool   `md:"enabled,required"`
	GrpcPort          int    `md:"grpcPort,required"`
	ProtoName         string `md:"protoName,required"`
	ProtoFile         string `md:"protoFile"`
	EnableTLS         bool   `md:"enableTLS"`
	ServerCert        string `md:"serverCert"`
	ServerKey         string `md:"serverKey"`
	EnableGrpcGateway bool   `md:"enableGrpcGateway"`
	HttpPort          int    `md:"httpPort"`
}

// FromMap method of Settings
func (s *Settings) FromMap(values map[string]interface{}) error {

	var (
		err error
	)

	s.Enabled, err = coerce.ToBool(values["enabled"])
	if err != nil {
		return err
	}

	s.GrpcPort, err = coerce.ToInt(values["grpcPort"])
	if err != nil {
		return err
	}

	s.ProtoName, err = coerce.ToString(values["protoName"])
	if err != nil {
		return err
	}

	s.ProtoFile, err = coerce.ToString(values["protoFile"])
	if err != nil {
		return err
	}

	s.EnableTLS, err = coerce.ToBool(values["enableTLS"])
	if err != nil {
		return err
	}

	s.ServerCert, err = coerce.ToString(values["serverCert"])
	if err != nil {
		return err
	}

	s.ServerKey, err = coerce.ToString(values["serverKey"])
	if err != nil {
		return err
	}

	s.EnableGrpcGateway, err = coerce.ToBool(values["enableGrpcGateway"])
	if err != nil {
		return err
	}

	s.HttpPort, err = coerce.ToInt(values["httpPort"])
	if err != nil {
		return err
	}

	return nil

}

type HandlerSettings struct {
	ServiceName string `md:"serviceName"`
	MethodName  string `md:"methodName"`
}

type Output struct {
	Params             map[string]interface{} `md:"params"`
	GrpcData           map[string]interface{} `md:"grpcData"`
	ProtobufRequestMap map[string]interface{} `md:"protobufRequestMap"`
}

func (o *Output) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"params":             o.Params,
		"grpcData":           o.GrpcData,
		"protobufRequestMap": o.ProtobufRequestMap,
	}
}

func (o *Output) FromMap(values map[string]interface{}) error {
	var err error
	o.Params, err = coerce.ToObject(values["params"])
	if err != nil {
		return err
	}
	o.GrpcData, err = coerce.ToObject(values["grpcData"])
	if err != nil {
		return err
	}
	o.ProtobufRequestMap, err = coerce.ToObject(values["protobufRequestMap"])
	if err != nil {
		return err
	}
	return nil
}

type Reply struct {
	Body interface{} `md:"body"`
}

func (r *Reply) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"body": r.Body,
	}
}

func (r *Reply) FromMap(values map[string]interface{}) error {
	r.Body = values["body"]
	return nil
}
