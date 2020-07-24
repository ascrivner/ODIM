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

//Package rfphandler ...
package rfphandler

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"sync"
	"time"

	iris "github.com/kataras/iris/v12"
	//"github.com/bharath-b-hpe/odimra/lib-utilities/common"
	pluginConfig "github.com/bharath-b-hpe/odimra/plugin-redfish/config"
	"github.com/bharath-b-hpe/odimra/plugin-redfish/rfpmodel"
	"github.com/bharath-b-hpe/odimra/plugin-redfish/rfpresponse"
	"github.com/bharath-b-hpe/odimra/plugin-redfish/rfputilities"
)

// GetPluginStatus defines the GetPluginStatus iris handler.
// and returns status
func GetPluginStatus(ctx iris.Context) {
	//Get token from Request
	token := ctx.GetHeader("X-Auth-Token")
	//Validating the token
	if token != "" {
		flag := TokenValidation(token)
		if !flag {
			log.Println("Invalid/Expired X-Auth-Token")
			ctx.StatusCode(http.StatusUnauthorized)
			ctx.WriteString("Invalid/Expired X-Auth-Token")
			return
		}
	}
	var messageQueueInfo []rfpresponse.EmbQueue
	var resp = rfpresponse.PluginStatusResponse{
		Comment: "Plugin Status Response",
		Name:    "Common Redfish Plugin Status",
		Version: "v0.1",
	}
	resp.Status = rfputilities.Status
	resp.Status.TimeStamp = time.Now().Format(time.RFC3339)
	resp.EventMessageBus = rfpresponse.EventMessageBus{
		EmbType: pluginConfig.Data.MessageBusConf.EmbType,
	}
	//messageQueueInfo := make([]rfpresponse.EmbQueue, 0)
	for i := 0; i < len(pluginConfig.Data.MessageBusConf.EmbQueue); i++ {
		messageQueueInfo = append(messageQueueInfo, rfpresponse.EmbQueue{
			QueueName: pluginConfig.Data.MessageBusConf.EmbQueue[i],
			QueueDesc: "Queue for redfish events",
		})
	}
	resp.EventMessageBus.EmbQueue = messageQueueInfo

	ctx.StatusCode(http.StatusOK)
	ctx.JSON(resp)

}

// GetPluginStartup ...
func GetPluginStartup(ctx iris.Context) {
	//Get token from Request
	token := ctx.GetHeader("X-Auth-Token")
	//Validating the token
	if token != "" {
		flag := TokenValidation(token)
		if !flag {
			log.Println("Invalid/Expired X-Auth-Token")
			ctx.StatusCode(http.StatusUnauthorized)
			ctx.WriteString("Invalid/Expired X-Auth-Token")
			return
		}
	}

	var startup []rfpmodel.Startup
	err := ctx.ReadJSON(&startup)
	if err != nil {
		log.Println("Error while trying to collect data from request: ", err)
		ctx.StatusCode(http.StatusBadRequest)
		ctx.WriteString("Error: bad request.")
		return
	}
	errorCh := make(chan error)
	startUpResponse := make(chan map[string]string)
	respBody := make(map[string]string)
	quit := make(chan bool)
	var writeWG sync.WaitGroup
	go func() {
		for {
			select {
			case err = <-errorCh:
				ctx.StatusCode(http.StatusInternalServerError)
				ctx.WriteString(err.Error())
				//close(startUpResponse)
				//close(respHeader)
				//close(errorCh)
				writeWG.Done()
				return
			case startResp := <-startUpResponse:
				for k, v := range startResp {
					respBody[k] = v
				}
				writeWG.Done()
			case <-quit:
				//close(startUpResponse)
				//close(respHeader)
				//close(errorCh)
				//close(quit)
				break
			}
		}
	}()
	for _, server := range startup {
		writeWG.Add(1)
		go checkCreateSub(server, startUpResponse, errorCh, &writeWG)
		//go checkCreateSub(server, startUpResponse, respHeader, errorCh)
	}
	writeWG.Wait()
	quit <- true
	ctx.StatusCode(http.StatusOK)
	ctx.JSON(respBody)
	return
}

func checkCreateSub(startup rfpmodel.Startup, startUpResponse chan map[string]string, errorCh chan error, writeWG *sync.WaitGroup) {
	var respBody = make(map[string]string)

	device := &rfputilities.RedfishDevice{
		Host:     startup.Device.Host,
		Username: startup.Device.Username,
		Password: string(startup.Device.Password),
		Location: startup.Location,
	}
	redfishClient, err := rfputilities.GetRedfishClient()
	if err != nil {
		errorCh <- err
		return
	}

	//Get Subscription details
	resp, err := redfishClient.GetSubscriptionDetail(device)
	if err != nil {
		errorCh <- err
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			errorCh <- err
			return
		}
		var obj rfpmodel.EvtSubPost
		if err = json.Unmarshal([]byte(body), &obj); err != nil {
			errorCh <- err
			return
		}

		res := reflect.DeepEqual(obj.EventTypes, startup.EventTypes)
		if !res {
			//Delete Subscription details
			resp, err := redfishClient.DeleteSubscriptionDetail(device)
			if err != nil {
				errorCh <- err
				return
			}
			defer resp.Body.Close()

			//Create new Subscription with details in odimra
			req := rfpmodel.EvtSubPost{
				Destination: "https://" + pluginConfig.Data.LoadBalancerConf.Host + ":" + pluginConfig.Data.LoadBalancerConf.Port + pluginConfig.Data.EventConf.DestURI,
				EventTypes:  startup.EventTypes,
				Context:     "Event Subscription",
				//      HTTPHeaders: reqPostBody.HTTPHeaders,
				Protocol: "Redfish",
			}
			device.PostBody, err = json.Marshal(req)
			if err != nil {
				errorCh <- err
				return
			}

			//Subscribe to Events
			resp, err = redfishClient.SubscribeForEvents(device)
			if err != nil {
				errorCh <- err
				return
			}
			defer resp.Body.Close()

		}

	} else if resp.StatusCode == http.StatusNotFound {
		req := rfpmodel.EvtSubPost{
			Destination: "https://" + pluginConfig.Data.LoadBalancerConf.Host + ":" + pluginConfig.Data.LoadBalancerConf.Port + pluginConfig.Data.EventConf.DestURI,
			EventTypes:  []string{"Alert"},
			Context:     "Event Subscription",
			//	HTTPHeaders: reqPostBody.HTTPHeaders,
			Protocol: "Redfish",
		}
		device.PostBody, err = json.Marshal(req)
		if err != nil {
			errorCh <- err
			return
		}

		//Subscribe to Events
		resp, err = redfishClient.SubscribeForEvents(device)
		if err != nil {
			errorCh <- err
			return
		}
		defer resp.Body.Close()
	}

	respBody[startup.Device.Host] = resp.Header.Get("location")
	startUpResponse <- respBody
	return
}
