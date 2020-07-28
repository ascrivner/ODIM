//(C) Copyright [2020] Hewlett Packard Enterprise Development LP
//
//Licensed under the Apache License, Version 2.0 (the "License"); you may
//not use this file except in compliance with the License. You may obtain
//a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//License for the specific language governing permissions and limitations
// under the License.
package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/ODIM-Project/ODIM/lib-utilities/common"
	"github.com/ODIM-Project/ODIM/lib-utilities/config"
	aggregatorproto "github.com/ODIM-Project/ODIM/lib-utilities/proto/aggregator"
	"github.com/ODIM-Project/ODIM/lib-utilities/response"
	"github.com/ODIM-Project/ODIM/svc-aggregation/agmodel"
)

var pluginContact = ExternalInterface{
	ContactClient:   mockContactClient,
	Auth:            mockIsAuthorized,
	CreateChildTask: mockCreateChildTask,
	UpdateTask:      mockUpdateTask,
	DecryptPassword: stubDevicePassword,
	GetPluginStatus: GetPluginStatusForTesting,
}

func mockCreateChildTask(sessionID, taskID string) (string, error) {
	switch taskID {
	case "taskWithoutChild":
		return "", fmt.Errorf("subtask cannot created")
	case "subTaskWithSlash":
		return "someSubTaskID/", nil
	default:
		return "someSubTaskID", nil
	}
}

func mockSystemData(systemID string) error {
	reqData, _ := json.Marshal(&map[string]interface{}{
		"Id": "1",
	})

	connPool, err := common.GetDBConnection(common.InMemory)
	if err != nil {
		return fmt.Errorf("error while trying to connecting to DB: %v", err.Error())
	}
	if err = connPool.Create("ComputerSystem", systemID, string(reqData)); err != nil {
		return fmt.Errorf("error while trying to create new %v resource: %v", "System", err.Error())
	}
	return nil
}

func mockUpdateTask(task common.TaskData) error {
	if task.TaskID == "invalid" {
		return fmt.Errorf("task with this ID not found")
	}
	return nil
}

func TestPluginContact_SetDefaultBootOrder(t *testing.T) {
	config.SetUpMockConfig(t)
	defer func() {
		err := common.TruncateDB(common.OnDisk)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		err = common.TruncateDB(common.InMemory)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	}()
	device1 := agmodel.Target{
		ManagerAddress: "100.0.0.1",
		Password:       []byte("imKp3Q6Cx989b6JSPHnRhritEcXWtaB3zqVBkSwhCenJYfgAYBf9FlAocE"),
		UserName:       "admin",
		DeviceUUID:     "24b243cf-f1e3-5318-92d9-2d6737d6b0b9",
		PluginID:       "GRF",
	}
	device2 := agmodel.Target{
		ManagerAddress: "100.0.0.2",
		Password:       []byte("imKp3Q6Cx989b6JSPHnRhritEcXWtaB3zqVBkSwhCenJYfgAYBf9FlAocE"),
		UserName:       "admin",
		DeviceUUID:     "7a2c6100-67da-5fd6-ab82-6870d29c7279",
		PluginID:       "GRF",
	}
	device3 := agmodel.Target{
		ManagerAddress: "100.0.0.2",
		Password:       []byte("passwordWithInvalidEncryption"),
		UserName:       "admin",
		DeviceUUID:     "7a2c6100-67da-5fd6-ab82-6870d29c7279",
		PluginID:       "GRF",
	}
	device4 := agmodel.Target{
		ManagerAddress: "100.0.0.2",
		Password:       []byte("someValidPassword"),
		UserName:       "admin",
		DeviceUUID:     "unknown-plugin-uuid",
		PluginID:       "Unknown-Plugin",
	}
	mockPluginData(t, "GRF")

	mockDeviceData("unreachable-server", device1)
	mockDeviceData("unknown-plugin-uuid", device4)
	mockDeviceData("123443cf-f1e3-5318-92d9-2d6737d65678", device3)
	mockDeviceData("7a2c6100-67da-5fd6-ab82-6870d29c7279", device2)
	mockDeviceData("24b243cf-f1e3-5318-92d9-2d6737d6b0b9", device1)

	mockSystemData("/redfish/v1/Systems/7a2c6100-67da-5fd6-ab82-6870d29c7279:1")
	mockSystemData("/redfish/v1/Systems/24b243cf-f1e3-5318-92d9-2d6737d6b0b9:1")
	mockSystemData("/redfish/v1/Systems/s83405033-67da-5fd6-ab82-458292935:1")
	mockSystemData("/redfish/v1/Systems/123443cf-f1e3-5318-92d9-2d6737d65678:1")
	mockSystemData("/redfish/v1/Systems/unknown-plugin-uuid:1")
	mockSystemData("/redfish/v1/Systems/unreachable-server:2")
	successArgs := response.Args{
		Code:    response.Success,
		Message: "Request completed successfully",
	}
	successResponse := successArgs.CreateGenericErrorResponse()
	invalidReqBodyResp := common.GeneralError(http.StatusInternalServerError, response.InternalError, "error while trying to set default boot order: invalid character 'i' looking for beginning of value", nil, nil)
	notFoundResp := common.GeneralError(http.StatusNotFound, response.ResourceNotFound, "one or more of the SetDefaultBootOrder requests failed. for more information please check SubTasks in URI: /redfish/v1/TaskService/Tasks/someID", []interface{}{"option", "SetDefaultBootOrder"}, nil)
	taskWithoutChildResp := common.GeneralError(http.StatusInternalServerError, response.InternalError, "one or more of the SetDefaultBootOrder requests failed. for more information please check SubTasks in URI: /redfish/v1/TaskService/Tasks/taskWithoutChild", nil, nil)
	generalInternalFailResp := common.GeneralError(http.StatusInternalServerError, response.InternalError, "one or more of the SetDefaultBootOrder requests failed. for more information please check SubTasks in URI: /redfish/v1/TaskService/Tasks/someID", nil, nil)
	type args struct {
		taskID, sessionUserName string
		req                     *aggregatorproto.AggregatorRequest
	}
	positiveReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/7a2c6100-67da-5fd6-ab82-6870d29c7279:1",
				"/redfish/v1/Systems/24b243cf-f1e3-5318-92d9-2d6737d6b0b9:1",
			},
		},
	})
	invalidUUIDReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/7a2c6100-67da-5fd6-ab82-6870d29c7279:1",
				"/redfish/v1/Systems/24b243cf-f1e3-5318-92d9-2d6737d6b0b:1",
			},
		},
	})
	invalidSystemReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/7a2c6100-67da-5fd6-ab82-6870d29c7279:1",
				"/redfish/v1/Systems/24b243cf-f1e3-5318-92d9-2d6737d6b0b9",
			},
		},
	})
	noUUIDInDBReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/s83405033-67da-5fd6-ab82-458292935:1",
			},
		},
	})
	decryptionFailReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/123443cf-f1e3-5318-92d9-2d6737d65678:1",
			},
		},
	})
	unknownPluginReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/unknown-plugin-uuid:1",
			},
		},
	})
	unreachableServerReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/unreachable-server:2",
			},
		},
	})
	tests := []struct {
		name string
		p    *ExternalInterface
		args args
		want response.RPC
	}{
		{
			name: "postive test Case",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  positiveReqData,
				},
			},
			want: response.RPC{
				StatusCode:    http.StatusOK,
				StatusMessage: response.Success,
				Header: map[string]string{
					"Cache-Control":     "no-cache",
					"Connection":        "keep-alive",
					"Content-type":      "application/json; charset=utf-8",
					"Transfer-Encoding": "chunked",
					"OData-Version":     "4.0",
				},
				Body: successResponse,
			},
		},
		{
			name: "invalid uuid id",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  invalidUUIDReqData,
				},
			},
			want: notFoundResp,
		},
		{
			name: "invalid system id",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  invalidSystemReqData,
				},
			},
			want: notFoundResp,
		},
		{
			name: "invalid request body",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  []byte("invalidData"),
				},
			},
			want: invalidReqBodyResp,
		},
		{
			name: "subtask creation failure",
			p:    &pluginContact,
			args: args{
				taskID: "taskWithoutChild", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  positiveReqData,
				},
			},
			want: taskWithoutChildResp,
		},
		{
			name: "no UUID in DB",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  noUUIDInDBReqData,
				},
			},
			want: notFoundResp,
		},
		{
			name: "decryption failure",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  decryptionFailReqData,
				},
			},
			want: generalInternalFailResp,
		},
		{
			name: "unknown plugin",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  unknownPluginReqData,
				},
			},
			want: notFoundResp,
		},
		{
			name: "unreachable server",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  unreachableServerReqData,
				},
			},
			want: generalInternalFailResp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.SetDefaultBootOrder(tt.args.taskID, tt.args.sessionUserName, tt.args.req); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExternalInterface.SetDefaultBootOrder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPluginContact_SetDefaultBootOrderForChildTaskWithSlash(t *testing.T) {
	config.SetUpMockConfig(t)
	defer func() {
		err := common.TruncateDB(common.OnDisk)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		err = common.TruncateDB(common.InMemory)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	}()
	device1 := agmodel.Target{
		ManagerAddress: "100.0.0.1",
		Password:       []byte("imKp3Q6Cx989b6JSPHnRhritEcXWtaB3zqVBkSwhCenJYfgAYBf9FlAocE"),
		UserName:       "admin",
		DeviceUUID:     "24b243cf-f1e3-5318-92d9-2d6737d6b0b9",
		PluginID:       "GRF",
	}
	mockPluginData(t, "GRF")
	mockDeviceData("24b243cf-f1e3-5318-92d9-2d6737d6b0b9", device1)
	mockSystemData("/redfish/v1/Systems/24b243cf-f1e3-5318-92d9-2d6737d6b0b9:1")
	successArgs := response.Args{
		Code:    response.Success,
		Message: "Request completed successfully",
	}
	successResponse := successArgs.CreateGenericErrorResponse()
	type args struct {
		taskID, sessionUserName string
		req                     *aggregatorproto.AggregatorRequest
	}
	positiveReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/24b243cf-f1e3-5318-92d9-2d6737d6b0b9:1",
			},
		},
	})
	tests := []struct {
		name string
		p    *ExternalInterface
		args args
		want response.RPC
	}{
		{
			name: "postive test Case with a slash",
			p:    &pluginContact,
			args: args{
				taskID: "subTaskWithSlash", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  positiveReqData,
				},
			},
			want: response.RPC{
				StatusCode:    http.StatusOK,
				StatusMessage: response.Success,
				Header: map[string]string{
					"Cache-Control":     "no-cache",
					"Connection":        "keep-alive",
					"Content-type":      "application/json; charset=utf-8",
					"Transfer-Encoding": "chunked",
					"OData-Version":     "4.0",
				},
				Body: successResponse,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.SetDefaultBootOrder(tt.args.taskID, tt.args.sessionUserName, tt.args.req); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExternalInterface.SetDefaultBootOrder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPluginContact_SetDefaultBootOrderWithXAuthToken(t *testing.T) {
	config.SetUpMockConfig(t)
	defer func() {
		err := common.TruncateDB(common.OnDisk)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		err = common.TruncateDB(common.InMemory)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
	}()
	device1 := agmodel.Target{
		ManagerAddress: "100.0.0.1",
		Password:       []byte("imKp3Q6Cx989b6JSPHnRhritEcXWtaB3zqVBkSwhCenJYfgAYBf9FlAocE"),
		UserName:       "admin",
		DeviceUUID:     "24b243cf-f1e3-5318-92d9-2d6737d6b0b9",
		PluginID:       "XAuthPlugin",
	}
	device2 := agmodel.Target{
		ManagerAddress: "100.0.0.1",
		Password:       []byte("imKp3Q6Cx989b6JSPHnRhritEcXWtaB3zqVBkSwhCenJYfgAYBf9FlAocE"),
		UserName:       "admin",
		DeviceUUID:     "xauth-fail-uuid",
		PluginID:       "XAuthPluginFail",
	}
	mockPluginData(t, "XAuthPlugin")
	mockPluginData(t, "XAuthPluginFail")
	mockDeviceData("xauth-fail-uuid", device2)
	mockDeviceData("24b243cf-f1e3-5318-92d9-2d6737d6b0b9", device1)
	mockSystemData("/redfish/v1/Systems/24b243cf-f1e3-5318-92d9-2d6737d6b0b9:1")
	mockSystemData("/redfish/v1/Systems/xauth-fail-uuid:1")
	successArgs := response.Args{
		Code:    response.Success,
		Message: "Request completed successfully",
	}
	successResponse := successArgs.CreateGenericErrorResponse()
	type args struct {
		taskID, sessionUserName string
		req                     *aggregatorproto.AggregatorRequest
	}
	positiveReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/24b243cf-f1e3-5318-92d9-2d6737d6b0b9:1",
			},
		},
	})
	failureReqData, _ := json.Marshal(SetBootOrderRequest{
		Parameters: BootOrderParameters{
			ServerCollection: []string{
				"/redfish/v1/Systems/xauth-fail-uuid:1",
			},
		},
	})
	xAuthFailResp := common.GeneralError(http.StatusUnauthorized, response.ResourceAtURIUnauthorized, "one or more of the SetDefaultBootOrder requests failed. for more information please check SubTasks in URI: /redfish/v1/TaskService/Tasks/someID", []interface{}{[]string{"/redfish/v1/Systems/xauth-fail-uuid:1"}}, nil)
	tests := []struct {
		name string
		p    *ExternalInterface
		args args
		want response.RPC
	}{
		{
			name: "postive test Case with XAuthToken",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  positiveReqData,
				},
			},
			want: response.RPC{
				StatusCode:    http.StatusOK,
				StatusMessage: response.Success,
				Header: map[string]string{
					"Cache-Control":     "no-cache",
					"Connection":        "keep-alive",
					"Content-type":      "application/json; charset=utf-8",
					"Transfer-Encoding": "chunked",
					"OData-Version":     "4.0",
				},
				Body: successResponse,
			},
		},
		{
			name: "XAuthToken failure",
			p:    &pluginContact,
			args: args{
				taskID: "someID", sessionUserName: "someUser",
				req: &aggregatorproto.AggregatorRequest{
					SessionToken: "validToken",
					RequestBody:  failureReqData,
				},
			},
			want: xAuthFailResp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.SetDefaultBootOrder(tt.args.taskID, tt.args.sessionUserName, tt.args.req); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExternalInterface.SetDefaultBootOrder() = %v, want %v", got, tt.want)
			}
		})
	}
}