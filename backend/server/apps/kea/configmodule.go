package kea

import (
	"context"

	pkgerrors "github.com/pkg/errors"
	keaconfig "isc.org/stork/appcfg/kea"
	keactrl "isc.org/stork/appctrl/kea"
	agentcomm "isc.org/stork/server/agentcomm"
	config "isc.org/stork/server/config"
	dbmodel "isc.org/stork/server/database/model"
)

// A configuration manager module responsible for Kea configuration.
type ConfigModule struct {
	// A configuration manager owning the module.
	manager config.ManagerAccessors
}

// Creates new instance of the Kea configuration module.
func NewConfigModule(manager config.ManagerAccessors) *ConfigModule {
	return &ConfigModule{
		manager: manager,
	}
}

// Commits the Kea configuration changes.
func (kea *ConfigModule) Commit(ctx context.Context) (context.Context, error) {
	var err error
	state, ok := ctx.Value(config.StateContextKey).(config.TransactionState)
	if !ok {
		return ctx, pkgerrors.Errorf("context lacks state")
	}
	for _, pu := range state.Updates {
		switch pu.Operation {
		case "host_add":
			ctx, err = kea.CommitHostAdd(ctx)
		default:
			err = pkgerrors.Errorf("unknown operation %s when called Commit()", pu.Operation)
		}
		if err != nil {
			return ctx, err
		}
	}
	return ctx, err
}

// Begins adding a new host reservation. Currently it is no-op but may evolve
// in the future.
func (kea *ConfigModule) BeginHostAdd(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

// Applies new host reservation. It prepares necessary commands to be sent
// to Kea upon commit.
func (kea *ConfigModule) ApplyHostAdd(ctx context.Context, host *dbmodel.Host) (context.Context, error) {
	if len(host.LocalHosts) == 0 {
		return ctx, pkgerrors.Errorf("applied host %d is not associated with any daemon", host.ID)
	}
	var commands []interface{}
	for _, lh := range host.LocalHosts {
		if lh.Daemon == nil {
			return ctx, pkgerrors.Errorf("applied host %d is associated with nil daemon", host.ID)
		}
		if lh.Daemon.App == nil {
			return ctx, pkgerrors.Errorf("applied host %d is associated with nil app", host.ID)
		}
		// Convert the host information to Kea reservation.
		reservation, err := keaconfig.CreateHostCmdsReservation(lh.DaemonID, host)
		if err != nil {
			return ctx, err
		}
		// Create command arguments.
		arguments := make(map[string]interface{})
		arguments["reservation"] = reservation
		// Associate the command with an app receiving this command.
		appCommand := make(map[string]interface{})
		appCommand["command"] = keactrl.NewCommand("reservation-add", []string{lh.Daemon.Name}, &arguments)
		appCommand["app"] = lh.Daemon.App
		commands = append(commands, appCommand)
	}
	var (
		daemonIDs []int64
		ok        bool
	)
	if daemonIDs, ok = ctx.Value(config.DaemonsContextKey).([]int64); !ok {
		return ctx, pkgerrors.New("context lacks daemons key")
	}
	// Create config update to be stored in the transaction state.
	update := config.NewUpdate("kea", "host_add", daemonIDs...)
	update.Recipe["commands"] = commands
	state := config.TransactionState{
		Updates: []*config.Update{
			update,
		},
	}
	ctx = context.WithValue(ctx, config.StateContextKey, state)
	return ctx, nil
}

// Create the host reservation in the Kea servers.
func (kea *ConfigModule) CommitHostAdd(ctx context.Context) (context.Context, error) {
	state, ok := ctx.Value(config.StateContextKey).(config.TransactionState)
	if !ok {
		return ctx, pkgerrors.New("context lacks state")
	}
	for _, update := range state.Updates {
		// Retrieve associations between the commands and apps.
		appCommands, ok := update.Recipe["commands"]
		if !ok {
			return ctx, pkgerrors.New("Kea commands not found in the context")
		}
		// Iterate over the associations.
		for _, acs := range appCommands.([]interface{}) {
			// Split the commands and apps.
			var (
				command keactrl.Command
				app     agentcomm.ControlledApp
			)
			if state.Scheduled {
				// If the context has been re-created after scheduling the config
				// change in the database we use a simplified structure holding the
				// App information.
				var commandApp struct {
					Command keactrl.Command
					App     config.App
				}
				if err := config.DecodeContextData(acs, &commandApp); err != nil {
					return ctx, err
				}
				app = commandApp.App
				command = commandApp.Command
			} else {
				// We didn't schedule the change so we have an original context
				// with an app represented using dbmodel.App structure.
				var commandApp struct {
					Command keactrl.Command
					App     dbmodel.App
				}
				if err := config.DecodeContextData(acs, &commandApp); err != nil {
					return ctx, err
				}
				app = commandApp.App
				command = commandApp.Command
			}
			// Send the command to Kea.
			var response keactrl.Response
			result, err := kea.manager.GetConnectedAgents().ForwardToKeaOverHTTP(context.Background(), app, []keactrl.SerializableCommand{command}, &response)
			if err != nil {
				return ctx, err
			}
			if err = result.GetFirstError(); err != nil {
				return ctx, err
			}
		}
	}
	return ctx, nil
}
