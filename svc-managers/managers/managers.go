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

//Package managers ...
package managers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	dmtf "github.com/bharath-b-hpe/odimra/lib-dmtf/model"
	"github.com/bharath-b-hpe/odimra/lib-utilities/common"
	"github.com/bharath-b-hpe/odimra/lib-utilities/config"
	"github.com/bharath-b-hpe/odimra/lib-utilities/errors"
	managersproto "github.com/bharath-b-hpe/odimra/lib-utilities/proto/managers"
	"github.com/bharath-b-hpe/odimra/lib-utilities/response"
	"github.com/bharath-b-hpe/odimra/svc-managers/mgrcommon"
	"github.com/bharath-b-hpe/odimra/svc-managers/mgrmodel"
	"github.com/bharath-b-hpe/odimra/svc-managers/mgrresponse"
)

// DeviceContact struct to inject the contact device function into the handlers
type DeviceContact struct {
	GetDeviceInfo         func(mgrcommon.ResourceInfoRequest) (string, error)
	ContactClient         func(string, string, string, string, interface{}, map[string]string) (*http.Response, error)
	DecryptDevicePassword func([]byte) ([]byte, error)
}

// GetManagersCollection will get the all the managers(odimra, Plugins, Servers)
func GetManagersCollection(req *managersproto.ManagerRequest) (response.RPC, error) {
	var resp response.RPC
	resp.Header = map[string]string{
		"Allow":             `"GET"`,
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"Content-type":      "application/json; charset=utf-8",
		"Transfer-Encoding": "chunked",
		"OData-Version":     "4.0",
	}
	managers := mgrresponse.ManagersCollection{
		OdataContext: "/redfish/v1/$metadata#ManagerCollection.ManagerCollection",
		OdataID:      "/redfish/v1/Managers",
		OdataType:    "#ManagerCollection.ManagerCollection",
		Description:  "Managers view",
		Name:         "Managers",
	}
	var members []dmtf.Link
	// Add odimra(self) as manager in manager collection
	oid := "/redfish/v1/Managers/" + config.Data.RootServiceUUID
	members = append(members, dmtf.Link{Oid: oid})

	// Add servers as manager in manager collection
	managersCollectionKeysArray, err := mgrmodel.GetAllKeysFromTable("Managers")
	if err != nil || len(managersCollectionKeysArray) == 0 {
		log.Printf("odimra Doesnt have Servers")
	}

	for _, key := range managersCollectionKeysArray {
		members = append(members, dmtf.Link{Oid: key})
	}
	managers.Members = members
	managers.MembersCount = len(members)
	resp.Body = managers
	resp.StatusCode = http.StatusOK
	return resp, nil
}

// GetManagers will fetch individual manager details with the given ID
func (d *DeviceContact) GetManagers(req *managersproto.ManagerRequest) response.RPC {
	var resp response.RPC
	resp.Header = map[string]string{
		"Allow":             `"GET"`,
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"Content-type":      "application/json; charset=utf-8",
		"Transfer-Encoding": "chunked",
		"OData-Version":     "4.0",
	}

	if req.ManagerID == config.Data.RootServiceUUID {
		manager, err := getManagerDetails(req.ManagerID)
		if err != nil {
			log.Printf("error getting manager details : %v", err.Error())
			errArgs := []interface{}{"Managers", req.ManagerID}
			errorMessage := err.Error()
			resp = common.GeneralError(http.StatusNotFound, response.ResourceNotFound, errorMessage,
				errArgs, nil)
			return resp
		}
		resp.Body = manager
	} else {

		requestData := strings.Split(req.ManagerID, ":")
		if len(requestData) <= 1 {
			resp = d.getPluginManagerResoure(requestData[0], req.URL)
			return resp
		}
		uuid := requestData[0]
		data, err := mgrmodel.GetManagerByURL(req.URL)
		if err != nil {
			log.Printf("error getting manager details : %v", err.Error())
			var errArgs = []interface{}{}
			var statusCode int
			var StatusMessage string
			errorMessage := err.Error()
			if errors.DBKeyNotFound == err.ErrNo() {
				errArgs = []interface{}{"Managers", req.ManagerID}

				statusCode = http.StatusNotFound
				StatusMessage = response.ResourceNotFound
			} else {
				statusCode = http.StatusInternalServerError
				StatusMessage = response.InternalError
			}
			resp = common.GeneralError(int32(statusCode), StatusMessage, errorMessage,
				errArgs, nil)
			return resp
		}
		var managerData map[string]interface{}
		jerr := json.Unmarshal([]byte(data), &managerData)
		if jerr != nil {
			errorMessage := "error unmarshalling manager details: " + jerr.Error()
			log.Println(errorMessage)
			resp = common.GeneralError(http.StatusInternalServerError, response.InternalError, errorMessage,
				nil, nil)
			return resp
		}
		// extracting the Manager Type from the  managerData
		var managerType string
		if val, ok := managerData["ManagerType"]; ok {
			managerType = val.(string)
		}

		if managerType != common.ManagerTypeService && managerType != "" {
			deviceData, err := d.getResourceInfoFromDevice(req.URL, uuid, requestData[1])
			if err != nil {
				log.Printf("warning: Device %v is unreachable: %v", req.URL, err)
				// Updating the state
				managerData["Status"] = map[string]string{
					"State": "Absent",
				}
			} else {
				jerr := json.Unmarshal([]byte(deviceData), &managerData)
				if jerr != nil {
					errorMessage := "error unmarshaling manager details: " + jerr.Error()
					log.Println(errorMessage)
					resp = common.GeneralError(http.StatusInternalServerError, response.InternalError, errorMessage,
						nil, nil)
					return resp
				}
			}
			err = mgrmodel.UpdateManagersData(req.URL, managerData)
			if err != nil {
				errorMessage := "error while saving manager details: " + err.Error()
				log.Println(errorMessage)
				resp = common.GeneralError(http.StatusInternalServerError, response.InternalError, errorMessage,
					nil, nil)
				return resp
			}
			dataBytes, err := json.Marshal(managerData)
			if err != nil {
				errorMessage := "error while marshalling manager details: " + err.Error()
				log.Println(errorMessage)
				resp = common.GeneralError(http.StatusInternalServerError, response.InternalError, errorMessage,
					nil, nil)
				return resp
			}
			data = string(dataBytes)
		}
		var resource map[string]interface{}
		json.Unmarshal([]byte(data), &resource)
		resp.Body = resource
	}
	resp.StatusCode = http.StatusOK
	resp.StatusMessage = response.Success
	return resp
}

func getManagerDetails(id string) (mgrmodel.Manager, error) {
	var mgr mgrmodel.Manager
	var name, managerType, firmwareVersion, managerid, uuid, state string

	mgrData, err := mgrmodel.GetManagerData(id)
	if err != nil {
		return mgr, err
	}
	name = mgrData.Name
	firmwareVersion = mgrData.FirmwareVersion
	managerType = mgrData.ManagerType
	managerid = mgrData.ID
	uuid = mgrData.UUID
	state = mgrData.State
	return mgrmodel.Manager{
		OdataContext:    "/redfish/v1/$metadata#Manager.Manager",
		OdataID:         "/redfish/v1/Managers/" + id,
		OdataType:       "#Manager.v1_3_3.Manager",
		Name:            name,
		ManagerType:     managerType,
		ID:              managerid,
		UUID:            uuid,
		FirmwareVersion: firmwareVersion,
		Status: &mgrmodel.Status{
			State: state,
		},
	}, nil
}

// GetManagersResource is used to fetch resource data. The function is supposed to be used as part of RPC
// For getting system resource information,  parameters need to be passed GetSystemsRequest .
// GetManagersResource holds the  Uuid,Url and Resourceid ,
// Url will be parsed from that search key will created
// There will be two return values for the fuction. One is the RPC response, which contains the
// status code, status message, headers and body and the second value is error.
func (d *DeviceContact) GetManagersResource(req *managersproto.ManagerRequest) response.RPC {
	var resp response.RPC
	resp.Header = map[string]string{
		"Allow":             `"GET"`,
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"Content-type":      "application/json; charset=utf-8",
		"Transfer-Encoding": "chunked",
		"OData-Version":     "4.0",
	}

	requestData := strings.Split(req.ManagerID, ":")
	if len(requestData) <= 1 {
		resp = d.getPluginManagerResoure(requestData[0], req.URL)
		return resp
	}
	uuid := requestData[0]
	urlData := strings.Split(req.URL, "/")
	var tableName string
	if req.ResourceID == "" {
		resourceName := urlData[len(urlData)-1]
		tableName = common.ManagersResource[resourceName]
	} else {
		tableName = urlData[len(urlData)-2]
	}

	data, err := mgrmodel.GetResource(tableName, req.URL)
	if err != nil {
		if errors.DBKeyNotFound == err.ErrNo() {
			var err error
			if data, err = d.getResourceInfoFromDevice(req.URL, uuid, requestData[1]); err != nil {
				errorMessage := "error while getting details from device: " + err.Error()
				log.Println(errorMessage)
				errArgs := []interface{}{tableName, req.ManagerID}
				return common.GeneralError(http.StatusNotFound, response.ResourceNotFound, errorMessage, errArgs, nil)
			}
		} else {
			errorMessage := "error getting managers details: " + err.Error()
			log.Println(errorMessage)
			return common.GeneralError(http.StatusInternalServerError, response.InternalError, errorMessage, []interface{}{}, nil)
		}
	}

	var resource map[string]interface{}
	json.Unmarshal([]byte(data), &resource)
	resp.Body = resource
	resp.StatusCode = http.StatusOK
	resp.StatusMessage = response.Success

	return resp
}
func (d *DeviceContact) getPluginManagerResoure(managerID, reqURI string) response.RPC {
	var resp response.RPC
	data, dberr := mgrmodel.GetManagerByURL("/redfish/v1/Managers/" + managerID)
	if dberr != nil {
		log.Printf("error getting manager details : %v", dberr.Error())
		var errArgs = []interface{}{"Managers", managerID}
		errorMessage := dberr.Error()
		resp = common.GeneralError(http.StatusNotFound, response.ResourceNotFound, errorMessage,
			errArgs, nil)
		return resp
	}
	var managerData map[string]interface{}
	jerr := json.Unmarshal([]byte(data), &managerData)
	if jerr != nil {
		errorMessage := "error unmarshalling manager details: " + jerr.Error()
		log.Println(errorMessage)
		resp = common.GeneralError(http.StatusInternalServerError, response.InternalError, errorMessage,
			nil, nil)
		return resp
	}
	var pluginID = managerData["Name"].(string)
	// Get the Plugin info
	plugin, gerr := mgrmodel.GetPluginData(pluginID)
	if gerr != nil {
		log.Printf("error getting manager details : %v", gerr.Error())
		var errArgs = []interface{}{"Plugin", pluginID}
		errorMessage := gerr.Error()
		resp = common.GeneralError(http.StatusNotFound, response.ResourceNotFound, errorMessage,
			errArgs, nil)
		return resp
	}
	var req mgrcommon.PluginContactRequest

	req.ContactClient = d.ContactClient
	req.Plugin = plugin

	if strings.EqualFold(plugin.PreferredAuthType, "XAuthToken") {
		token := mgrcommon.GetPluginToken(req)
		if token == "" {
			var errorMessage = "error: Unable to create session with plugin " + plugin.ID
			return common.GeneralError(http.StatusUnauthorized, response.NoValidSession, errorMessage,
				[]interface{}{}, nil)
		}
		req.Token = token
	} else {
		req.BasicAuth = map[string]string{
			"UserName": plugin.Username,
			"Password": string(plugin.Password),
		}

	}
	req.OID = reqURI
	var errorMessage = "error while getting the details " + reqURI + ": "
	var header = map[string]string{"Content-type": "application/json; charset=utf-8"}
	body, _, getResponse, err := mgrcommon.ContactPlugin(req, errorMessage)
	if err != nil {
		if getResponse.StatusCode == http.StatusUnauthorized && strings.EqualFold(req.Plugin.PreferredAuthType, "XAuthToken") {
			if body, _, getResponse, err = mgrcommon.RetryManagersOperation(req, errorMessage); err != nil {
				resp.StatusCode = getResponse.StatusCode
				json.Unmarshal(body, &resp.Body)
				resp.Header = header
				return resp
			}
		} else {
			resp.StatusCode = getResponse.StatusCode
			json.Unmarshal(body, &resp.Body)
			resp.Header = header
			return resp
		}
	}
	return fillResponse(body)

}

func fillResponse(body []byte) response.RPC {
	var resp response.RPC
	data := string(body)
	//replacing the resposne with north bound translation URL
	for key, value := range config.Data.URLTranslation.NorthBoundURL {
		data = strings.Replace(data, key, value, -1)
	}
	var respData map[string]interface{}
	err := json.Unmarshal([]byte(data), &respData)
	if err != nil {
		log.Printf(err.Error())
		return common.GeneralError(http.StatusInternalServerError, response.InternalError, err.Error(),
			[]interface{}{}, nil)
	}
	resp.Body = respData
	resp.Header = map[string]string{
		"Allow":             `"GET"`,
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"Content-type":      "application/json; charset=utf-8",
		"Transfer-Encoding": "chunked",
		"OData-Version":     "4.0",
	}
	resp.StatusCode = http.StatusOK
	resp.StatusMessage = response.Success
	return resp

}

func (d *DeviceContact) getResourceInfoFromDevice(reqURL, uuid, systemID string) (string, error) {
	var getDeviceInfoRequest = mgrcommon.ResourceInfoRequest{
		URL:                   reqURL,
		UUID:                  uuid,
		SystemID:              systemID,
		ContactClient:         d.ContactClient,
		DecryptDevicePassword: d.DecryptDevicePassword,
	}
	return d.GetDeviceInfo(getDeviceInfoRequest)

}
