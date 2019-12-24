package agentcomm

import (
	"net"
	"time"
	"strconv"
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/pkg/errors"

	"isc.org/stork"
	"isc.org/stork/api"
)

type KeaDaemon struct {
	Pid int32
	Name string
	Active bool
	Version string
	ExtendedVersion string
}

type AppCommon struct {
	Version string
	CtrlPort int64
	Active bool
}

type AppKea struct {
	AppCommon
	ExtendedVersion string
	Daemons []KeaDaemon
}

type AppBind struct {
	AppCommon
}

// State of the machine. It describes multiple properties of the machine like number of CPUs
// or operating system name and version.
type State struct {
	Address string
	AgentVersion string
	Cpus int64
	CpusLoad string
	Memory int64
	Hostname string
	Uptime int64
	UsedMemory int64
	Os string
	Platform string
	PlatformFamily string
	PlatformVersion string
	KernelVersion string
	KernelArch string
	VirtualizationSystem string
	VirtualizationRole string
	HostID string
	LastVisited time.Time
	Error string
	Apps []interface{}
}

// Get version from agent.
func (agents *connectedAgentsData) GetState(ctx context.Context, address string, agentPort int64) (*State, error) {
	// Find agent in map.
	addrPort := net.JoinHostPort(address, strconv.FormatInt(agentPort, 10))
	agent, err := agents.GetConnectedAgent(addrPort)
	if err != nil {
		return nil, err
	}

	// Call agent for version.
	grpcState, err := agent.Client.GetState(ctx, &agentapi.GetStateReq{})
	if err != nil {
		// reconnect and try again
		err2 := agent.MakeGrpcConnection()
		if err2 != nil {
			log.Warn(err)
			return nil, errors.Wrap(err2, "problem with connection to agent")
		}
		grpcState, err = agent.Client.GetState(ctx, &agentapi.GetStateReq{})
		if err != nil {
			return nil, errors.Wrap(err, "problem with connection to agent")
		}
	}


	var apps []interface{}
	for _, srv := range grpcState.Apps {

		switch s := srv.App.(type) {
		case *agentapi.App_Kea:
			log.Printf("s.Kea.Daemons %+v", s.Kea.Daemons)
			var daemons []KeaDaemon
			for _, d := range s.Kea.Daemons {
				daemons = append(daemons, KeaDaemon{
					Pid: d.Pid,
					Name: d.Name,
					Active: d.Active,
					Version: d.Version,
					ExtendedVersion: d.ExtendedVersion,
				})
			}
			apps = append(apps, &AppKea{
				AppCommon: AppCommon{
					Version: srv.Version,
					CtrlPort: srv.CtrlPort,
					Active: srv.Active,
				},
				ExtendedVersion: s.Kea.ExtendedVersion,
				Daemons: daemons,
			})
		case *agentapi.App_Bind:
			log.Println("NOT IMPLEMENTED")
		default:
			log.Println("unsupported app type")
		}
	}

	state := State{
		Address: address,
		AgentVersion: grpcState.AgentVersion,
		Cpus: grpcState.Cpus,
		CpusLoad: grpcState.CpusLoad,
		Memory: grpcState.Memory,
		Hostname: grpcState.Hostname,
		Uptime: grpcState.Uptime,
		UsedMemory: grpcState.UsedMemory,
		Os: grpcState.Os,
		Platform: grpcState.Platform,
		PlatformFamily: grpcState.PlatformFamily,
		PlatformVersion: grpcState.PlatformVersion,
		KernelVersion: grpcState.KernelVersion,
		KernelArch: grpcState.KernelArch,
		VirtualizationSystem: grpcState.VirtualizationSystem,
		VirtualizationRole: grpcState.VirtualizationRole,
		HostID: grpcState.HostID,
		LastVisited: stork.UTCNow(),
		Error: grpcState.Error,
		Apps: apps,
	}

	return &state, nil
}

// Forwards a Kea command via the Stork Agent and Kea Control Agent and then
// parses the response.
func (agents *connectedAgentsData) ForwardToKeaOverHttp(ctx context.Context, url string, command *KeaCommand, address string, agentPort int64) (*KeaResponseList, error) {
	// Find the agent by address and port.
	addrPort := net.JoinHostPort(address, strconv.FormatInt(agentPort, 10))
	agent, err := agents.GetConnectedAgent(addrPort)
	if err != nil {
		err = errors.Wrapf(err, "there is no agent available at address %s:%d", address, agentPort)
		return nil, err
	}

	// Prepare the on-wire representation of the command.
	c := command.Marshal()

	req := &agentapi.ForwardToKeaOverHttpReq{
		Url:        url,
		KeaRequest: c,
	}

	// Send the command to the Stork agent.
	rsp, err := agent.Client.ForwardToKeaOverHttp(ctx, req)
	if err != nil {
		err = errors.Wrapf(err, "failed to forward Kea command to %s, command was: %s", url, c)
		return nil, err
	}

	// Try to parse the response from the on-wire format.
	keaResponseList, err := UnmarshalKeaResponseList(command, rsp.GetKeaResponse())
	if err != nil {
		err = errors.Wrapf(err, "failed to parse Kea response from %s, response was: %s", url, rsp.GetKeaResponse())
		return nil, err
	}

	// Everything was fine, so return the response and no error.
	return keaResponseList, nil
}
