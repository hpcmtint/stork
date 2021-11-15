package dumps

import (
	"context"
	"fmt"
	"strings"

	dbmodel "isc.org/stork/server/database/model"
	"isc.org/stork/server/gen/models"
)

// The dump of the all fetchable logs.
// It means that it dumps the log tails from each log target
// related to the machine except stdin/stdout/syslog targets.
type LogsDump struct {
	BasicDump
	machine    *dbmodel.Machine
	logSources LogTailSource
}

// Log tail source - it corresponds to agentcomm.ConnectedlogSources interface.
// It is needed to avoid the dependency cycle.
type LogTailSource interface {
	TailTextFile(ctx context.Context, agentAddress string, agentPort int64, path string, offset int64) ([]string, error)
}

func NewLogsDump(machine *dbmodel.Machine, logSources LogTailSource) *LogsDump {
	return &LogsDump{
		*NewBasicDump("logs"),
		machine, logSources,
	}
}

func (d *LogsDump) Execute() error {
	for _, app := range d.machine.Apps {
		for _, daemon := range app.Daemons {
			for logTargetID, logTarget := range daemon.LogTargets {
				if logTarget.Output == "stdout" || logTarget.Output == "stderr" ||
					strings.HasPrefix(logTarget.Output, "syslog") {
					continue
				}

				contents, err := d.logSources.TailTextFile(
					context.Background(),
					d.machine.Address,
					d.machine.AgentPort,
					logTarget.Output,
					4000)

				var errStr string
				if err != nil {
					errStr = err.Error()
				}

				tail := &models.LogTail{
					Machine: &models.AppMachine{
						ID:       d.machine.ID,
						Address:  d.machine.Address,
						Hostname: d.machine.State.Hostname,
					},
					AppID:           app.ID,
					AppName:         app.Name,
					AppType:         app.Type,
					LogTargetOutput: logTarget.Output,
					Contents:        contents,
					Error:           errStr,
				}

				name := fmt.Sprintf("a-%d-%s_d-%d-%s_t-%d-%s",
					app.ID, app.Name,
					daemon.ID, daemon.Name,
					logTargetID, logTarget.Name)

				d.AppendArtifact(NewBasicStructArtifact(
					name, tail,
				))
			}
		}
	}

	return nil
}