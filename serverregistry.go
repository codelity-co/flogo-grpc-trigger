package grpc

import (

	//used for generated stub files

	"context"
	_ "github.com/golang/protobuf/proto"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

// ServiceRegistery holds all the server services written in proto file
var ServiceRegistery = NewServiceRegistry()

// ServerService methods to invoke registartion of service
type ServerService interface {
	ServiceInfo() *ServiceInfo
	RunRegisterServerService(s *grpc.Server, t *Trigger)
	RegisterHttpMuxHandler(ctx context.Context, mux *runtime.ServeMux)
}

// ServiceInfo holds name of service and name of proto
type ServiceInfo struct {
	ServiceName string
	ProtoName   string
}

// ServiceRegistry data structure to hold the services
type ServiceRegistry struct {
	ServerServices map[string]ServerService
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{ServerServices: make(map[string]ServerService)}
}

// RegisterServerService resgisters server services
func (sr *ServiceRegistry) RegisterServerService(service ServerService) {
	sr.ServerServices[service.ServiceInfo().ProtoName+service.ServiceInfo().ServiceName] = service
}
