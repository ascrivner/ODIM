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
package main

import (
	"log"
	"os"

	dc "github.com/bharath-b-hpe/odimra/lib-messagebus/datacommunicator"
	"github.com/bharath-b-hpe/odimra/lib-utilities/common"
	"github.com/bharath-b-hpe/odimra/lib-utilities/config"
	aggregatorproto "github.com/bharath-b-hpe/odimra/lib-utilities/proto/aggregator"
	"github.com/bharath-b-hpe/odimra/lib-utilities/services"
	"github.com/bharath-b-hpe/odimra/svc-aggregation/agcommon"
	"github.com/bharath-b-hpe/odimra/svc-aggregation/agmessagebus"
	"github.com/bharath-b-hpe/odimra/svc-aggregation/rpc"
	"github.com/bharath-b-hpe/odimra/svc-aggregation/system"
	"github.com/bharath-b-hpe/odimra/svc-plugin-rest-client/pmbhandle"
)

// Schema is a struct to define search, condition and query keys
type Schema struct {
	SearchKeys    []string `json:"searchKeys"`
	ConditionKeys []string `json:"conditionKeys"`
	QueryKeys     []string `json:"queryKeys"`
}

func main() {
	// verifying the uid of the user
	if uid := os.Geteuid(); uid == 0 {
		log.Fatal("Aggregation Service should not be run as the root user")
	}
	if err := config.SetConfiguration(); err != nil {
		log.Fatalf("error while trying to set configuration: %v", err)
	}

	if err := dc.SetConfiguration(config.Data.MessageQueueConfigFilePath); err != nil {
		log.Fatalf("error while trying to set messagebus configuration: %v", err)
	}

	if err := common.CheckDBConnection(); err != nil {
		log.Fatalf("error while trying to check DB connection health: %v", err)
	}

	//initialize global record used for tracking ongoing requests
	system.ActiveReqSet.ReqRecord = make(map[string]interface{})

	err := services.InitializeService(services.Aggregator)
	if err != nil {
		log.Fatalf("fatal: error while trying to initialize service: %v", err)
	}

	aggregator := rpc.GetAggregator()
	aggregatorproto.RegisterAggregatorHandler(services.Service.Server(), aggregator)

	// Rediscover the Resources by looking in OnDisk DB, populate the resources in InMemory DB
	//This happens only if the InMemory DB lost it contents due to DB reboot or host VM reboot.
	p := system.ExternalInterface{
		ContactClient:   pmbhandle.ContactPlugin,
		Auth:            services.IsAuthorized,
		PublishEventMB:  agmessagebus.Publish,
		GetPluginStatus: agcommon.GetPluginStatus,
		SubscribeToEMB:  services.SubscribeToEMB,
		DecryptPassword: common.DecryptWithPrivateKey,
		UpdateTask:      system.UpdateTaskData,
	}
	go p.RediscoverResources()

	if err = services.Service.Run(); err != nil {
		log.Fatalf("failed to run a service: %v", err)
	}

}
