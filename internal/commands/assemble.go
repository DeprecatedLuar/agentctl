package commands

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/DeprecatedLuar/agentctl/internal"
	"github.com/DeprecatedLuar/agentctl/internal/config"
	"github.com/DeprecatedLuar/agentctl/internal/gateways"
	"github.com/DeprecatedLuar/agentctl/internal/gateways/cli"
	"github.com/DeprecatedLuar/agentctl/internal/gateways/telegram"
	"github.com/DeprecatedLuar/agentctl/internal/logger"
	"github.com/DeprecatedLuar/agentctl/internal/providers/audio"
	"github.com/DeprecatedLuar/agentctl/internal/session"
)

// AssembledOrchestrator bundles everything a caller needs to drive a
// conversation turn (and, for daemon callers, host gateways): the domain
// Orchestrator, its OutboundDispatcher, and the gateway Interface instances
// ready to Start(ctx).
type AssembledOrchestrator struct {
	Orchestrator *internal.Orchestrator
	Store        session.SessionStore
	Dispatcher   *gateways.OutboundDispatcher
	Gateways     []internal.Gateway
	GatewayNames []string
}

// assembleOrchestrator builds the session store, Orchestrator, and
// OutboundDispatcher, and registers a Sender for each requested gateway
// name. It performs no I/O beyond gateway construction (no locking, no
// signal handling, no goroutines) - those remain the caller's concern.
//
// "cli" is a valid access identity but not a daemon-hosted gateway (no
// persistent listener - one-shot chat calls the Orchestrator directly), so
// it produces no entry in Gateways/GatewayNames.
func assembleOrchestrator(absPath string, transcriber audio.Transcriber, lg *slog.Logger, verbose, debug bool, gatewayNames []string) (*AssembledOrchestrator, error) {
	store := session.NewJSONLStore(absPath)

	orch := &internal.Orchestrator{
		AgentFolder:  absPath,
		SessionStore: store,
		Logger:       lg,
		Verbose:      verbose,
		Debug:        debug,
	}

	dispatcher := gateways.NewOutboundDispatcher()
	dispatcher.Register(cli.NewSender())

	var gatewayList []internal.Gateway
	var actualNames []string
	for _, name := range gatewayNames {
		switch name {
		case gatewayCLI:
			continue
		case gatewayTelegram:
			telegramGateway, err := telegram.NewTelegram(absPath, transcriber, orch, store, lg, verbose)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize telegram gateway: %w", err)
			}
			dispatcher.Register(telegramGateway)
			gatewayList = append(gatewayList, telegramGateway)
			actualNames = append(actualNames, name)
		default:
			return nil, fmt.Errorf("unknown gateway: %s", name)
		}
	}

	orch.Dispatcher = dispatcher

	return &AssembledOrchestrator{
		Orchestrator: orch,
		Store:        store,
		Dispatcher:   dispatcher,
		Gateways:     gatewayList,
		GatewayNames: actualNames,
	}, nil
}

// loadAgentAndLogger loads agent.toml and builds a logger for a one-shot
// command (chat/deliver). Unlike HandleServe's startup validation, per-request
// config/tool/prompt errors surface naturally through Orchestrator.handleMessage's
// own hot-reload load - this only needs enough of agentCfg to size the logger.
func loadAgentAndLogger(absPath string, debug bool) (*config.AgentConfig, *slog.Logger, error) {
	agentCfg, issues := config.LoadAgent(absPath)
	if agentCfg == nil {
		var msgs []string
		for _, issue := range issues {
			msgs = append(msgs, issue.Message)
		}
		return nil, nil, fmt.Errorf("failed to load agent configuration: %v", msgs)
	}

	enableFileLogging := agentCfg.Agent.LoggingEnabled() || debug
	logDir := filepath.Join(absPath, dataDir, logsDir)
	lg, err := logger.SetupOneShot(logDir, debug, enableFileLogging)
	if err != nil {
		return nil, nil, fmt.Errorf("setup logger: %w", err)
	}

	return agentCfg, lg, nil
}

// deliveryGateways extracts the unique gateway names referenced by a
// --deliver channel list (e.g. "telegram@123" -> "telegram", "telegram" ->
// "telegram", matching deliverToChannels' own parsing in orchestration.go),
// so one-shot commands only construct the Senders they actually need
// instead of every configured gateway.
func deliveryGateways(channels []string) []string {
	seen := make(map[string]bool)
	var names []string
	for _, ch := range channels {
		name := ch
		if strings.Contains(ch, "@") {
			name = strings.SplitN(ch, "@", 2)[1]
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}
