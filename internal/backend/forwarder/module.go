// module.go assembles the forwarder service into a legacy HTTP/Connect handler.
package forwarder

import (
	"net/http"

	"connectrpc.com/connect"

	modeladapter "cursor/internal/backend/agent/model"
)

type Module struct {
	Service                  *Service
	LocalBidiHandler         http.Handler
	LocalRunSSE              http.Handler
	AiHandler                http.Handler
	RepositoryServiceHandler http.Handler
	UploadServiceHandler     http.Handler
}

// NewModule creates the forwarder module and exports the local Bidi / RunSSE handlers.
func NewModule(historyRoot string, channelService modeladapter.ChannelResolver) *Module {
	service := NewService(historyRoot, channelService)
	legacyBidiAppendProcedure := "/aiserver.v1.BidiService/BidiAppend"
	legacyRunSSEProcedure := "/agent.v1.AgentService/RunSSE"
	return &Module{
		Service:                  service,
		LocalBidiHandler:         connect.NewUnaryHandler(legacyBidiAppendProcedure, service.BidiAppend),
		LocalRunSSE:              NewLegacyRunSSEHandler(legacyRunSSEProcedure, service.RunSSE),
		AiHandler:                newAIHandler(service),
		RepositoryServiceHandler: newRepositoryServiceHandler(service),
		UploadServiceHandler:     newUploadServiceHandler(service),
	}
}
