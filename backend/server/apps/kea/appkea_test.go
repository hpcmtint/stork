package kea

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	//log "github.com/sirupsen/logrus"

	"isc.org/stork/server/agentcomm"
	dbmodel "isc.org/stork/server/database/model"
	storktest "isc.org/stork/server/test"
)

// Kea servers' response to config-get command from CA. The argument indicates if
// it is a response from a single server or two servers.
func mockGetConfigFromCAResponse(daemons int, cmdResponses []interface{}) {
	list1 := cmdResponses[0].(*[]VersionGetResponse)
	*list1 = []VersionGetResponse{
		{
			KeaResponseHeader: agentcomm.KeaResponseHeader{
				Result: 0,
				Daemon: "ca",
			},
			Arguments: &VersionGetRespArgs{
				Extended: "Extended version",
			},
		},
	}
	list2 := cmdResponses[1].(*[]CAConfigGetResponse)
	*list2 = []CAConfigGetResponse{
		{
			KeaResponseHeader: agentcomm.KeaResponseHeader{
				Result: 0,
				Daemon: "ca",
			},
			Arguments: &CAConfigGetRespArgs{
				ControlAgent: &ControlAgentData{
					ControlSockets: &ControlSocketsData{
						Dhcp4: &SocketData{
							SocketName: "aaaa",
							SocketType: "unix",
						},
					},
				},
			},
		},
	}
	if daemons > 1 {
		(*list2)[0].Arguments.ControlAgent.ControlSockets.Dhcp6 = &SocketData{
			SocketName: "bbbb",
			SocketType: "unix",
		}
	}
}

// Kea servers' response to config-get command from other Kea daemons. The argument indicates if
// it is a response from a single server or two servers.
func mockGetConfigFromOtherDaemonsResponse(daemons int, cmdResponses []interface{}) {
	// version-get response
	list1 := cmdResponses[0].(*[]VersionGetResponse)
	*list1 = []VersionGetResponse{
		{
			KeaResponseHeader: agentcomm.KeaResponseHeader{
				Result: 0,
				Daemon: "dhcp4",
			},
			Arguments: &VersionGetRespArgs{
				Extended: "Extended version",
			},
		},
	}
	if daemons > 1 {
		*list1 = append(*list1, VersionGetResponse{
			KeaResponseHeader: agentcomm.KeaResponseHeader{
				Result: 0,
				Daemon: "dhcp6",
			},
			Arguments: &VersionGetRespArgs{
				Extended: "Extended version",
			},
		})
	}
	// status-get response
	list2 := cmdResponses[1].(*[]StatusGetResponse)
	*list2 = []StatusGetResponse{
		{
			KeaResponseHeader: agentcomm.KeaResponseHeader{
				Result: 0,
				Daemon: "dhcp4",
			},
			Arguments: &StatusGetRespArgs{
				Pid: 123,
			},
		},
	}
	if daemons > 1 {
		*list2 = append(*list2, StatusGetResponse{
			KeaResponseHeader: agentcomm.KeaResponseHeader{
				Result: 0,
				Daemon: "dhcp6",
			},
			Arguments: &StatusGetRespArgs{
				Pid: 123,
			},
		})
	}
	// config-get response
	list3 := cmdResponses[2].(*[]agentcomm.KeaResponse)
	*list3 = []agentcomm.KeaResponse{
		{
			KeaResponseHeader: agentcomm.KeaResponseHeader{
				Result: 0,
				Daemon: "dhcp4",
			},
			Arguments: &map[string]interface{}{
				"Dhcp4": map[string]interface{}{
					"hooks-libraries": []interface{}{
						map[string]interface{}{
							"library": "hook_abc.so",
						},
						map[string]interface{}{
							"library": "hook_def.so",
						},
					},
				},
			},
		},
	}
	if daemons > 1 {
		*list3 = append(*list3, agentcomm.KeaResponse{
			KeaResponseHeader: agentcomm.KeaResponseHeader{
				Result: 0,
				Daemon: "dhcp6",
			},
			Arguments: &map[string]interface{}{
				"Dhcp6": map[string]interface{}{
					"hooks-libraries": []interface{}{
						map[string]interface{}{
							"library": "hook_abc.so",
						},
						map[string]interface{}{
							"library": "hook_def.so",
						},
					},
				},
			},
		})
	}
}

// Check if GetAppState returns response to the forwarded command.
func TestGetAppStateWith1Daemon(t *testing.T) {
	ctx := context.Background()

	// check getting config of 1 daemon
	fa := storktest.NewFakeAgents(func(callNo int, cmdResponses []interface{}) {
		if callNo == 0 {
			mockGetConfigFromCAResponse(1, cmdResponses)
		} else if callNo == 1 {
			mockGetConfigFromOtherDaemonsResponse(1, cmdResponses)
		}
	})

	dbApp := dbmodel.App{
		CtrlAddress: "192.0.2.0",
		CtrlPort:    1234,
		Machine: &dbmodel.Machine{
			Address:   "192.0.2.0",
			AgentPort: 1111,
		},
	}

	GetAppState(ctx, fa, &dbApp)

	require.Equal(t, "http://192.0.2.0:1234/", fa.RecordedURL)
	require.Equal(t, "version-get", fa.RecordedCommands[0])
	require.Equal(t, "config-get", fa.RecordedCommands[1])
}

func TestGetAppStateWith2Daemons(t *testing.T) {
	ctx := context.Background()

	// check getting configs of 2 daemons
	fa := storktest.NewFakeAgents(func(callNo int, cmdResponses []interface{}) {
		if callNo == 0 {
			mockGetConfigFromCAResponse(2, cmdResponses)
		} else if callNo == 1 {
			mockGetConfigFromOtherDaemonsResponse(2, cmdResponses)
		}
	})

	dbApp := dbmodel.App{
		CtrlAddress: "192.0.2.0",
		CtrlPort:    1234,
		Machine: &dbmodel.Machine{
			Address:   "192.0.2.0",
			AgentPort: 1111,
		},
	}

	GetAppState(ctx, fa, &dbApp)

	require.Equal(t, "http://192.0.2.0:1234/", fa.RecordedURL)
	require.Equal(t, "version-get", fa.RecordedCommands[0])
	require.Equal(t, "config-get", fa.RecordedCommands[1])
}

// Check if GetDaemonHooks returns hooks for given daemon.
func TestGetDaemonHooksFrom1Daemon(t *testing.T) {
	dbApp := dbmodel.App{
		Details: dbmodel.AppKea{
			Daemons: []*dbmodel.KeaDaemon{
				{
					Name: "dhcp4",
					Config: &map[string]interface{}{
						"Dhcp4": map[string]interface{}{
							"hooks-libraries": []interface{}{
								map[string]interface{}{
									"library": "hook_abc.so",
								},
							},
						},
					},
				},
			},
		},
	}

	hooksMap := GetDaemonHooks(&dbApp)
	require.NotNil(t, hooksMap)
	hooks, ok := hooksMap["dhcp4"]
	require.True(t, ok)
	require.Len(t, hooks, 1)
	require.Equal(t, "hook_abc.so", hooks[0])
}

// Check getting hooks of 2 daemons
func TestGetDaemonHooksFrom2Daemons(t *testing.T) {
	dbApp := dbmodel.App{
		Details: dbmodel.AppKea{
			Daemons: []*dbmodel.KeaDaemon{
				{
					Name: "dhcp6",
					Config: &map[string]interface{}{
						"Dhcp6": map[string]interface{}{
							"hooks-libraries": []interface{}{
								map[string]interface{}{
									"library": "hook_abc.so",
								},
								map[string]interface{}{
									"library": "hook_def.so",
								},
							},
						},
					},
				},
				{
					Name: "dhcp4",
					Config: &map[string]interface{}{
						"Dhcp4": map[string]interface{}{
							"hooks-libraries": []interface{}{
								map[string]interface{}{
									"library": "hook_abc.so",
								},
							},
						},
					},
				},
			},
		},
	}

	hooksMap := GetDaemonHooks(&dbApp)
	require.NotNil(t, hooksMap)
	hooks, ok := hooksMap["dhcp4"]
	require.True(t, ok)
	require.Len(t, hooks, 1)
	require.Equal(t, "hook_abc.so", hooks[0])
	hooks, ok = hooksMap["dhcp6"]
	require.True(t, ok)
	require.Len(t, hooks, 2)
	require.Contains(t, hooks, "hook_abc.so")
	require.Contains(t, hooks, "hook_def.so")
}
