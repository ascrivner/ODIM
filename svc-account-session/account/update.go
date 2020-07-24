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

// Package account ...
package account

// ---------------------------------------------------------------------------------------
// IMPORT Section
// ---------------------------------------------------------------------------------------
import (
	"encoding/base64"
	"github.com/bharath-b-hpe/odimra/lib-utilities/common"
	"github.com/bharath-b-hpe/odimra/lib-utilities/errors"
	accountproto "github.com/bharath-b-hpe/odimra/lib-utilities/proto/account"
	"github.com/bharath-b-hpe/odimra/lib-utilities/response"
	"github.com/bharath-b-hpe/odimra/svc-account-session/asmodel"
	"github.com/bharath-b-hpe/odimra/svc-account-session/asresponse"
	"golang.org/x/crypto/sha3"
	"log"
	"net/http"
)

// Update defines the updation of the account details. Every account details can be
// updated other than the UserName if the session parameter have sufficient privileges.
//
// For updating an account, two parameters need to be passed UpdateAccountRequest and Session.
// New Password and RoleId will be part of UpdateAccountRequest,
// and Session parameter will have all session related data, espically the privileges.
//
// Output is the RPC response, which contains the status code, status message, headers and body.
func Update(req *accountproto.UpdateAccountRequest, session *asmodel.Session) response.RPC {
	commonResponse := response.Response{
		OdataType:    "#ManagerAccount.v1_4_0.ManagerAccount",
		OdataID:      "/redfish/v1/AccountService/Accounts/" + req.AccountID,
		OdataContext: "/redfish/v1/$metadata#ManagerAccount.ManagerAccount",
		ID:           req.AccountID,
		Name:         "Account Service",
	}

	var (
		resp response.RPC
		err  error
	)

	requestUser := asmodel.User{
		UserName:     req.UserName,
		Password:     req.Password,
		RoleID:       req.RoleId,
		AccountTypes: []string{"Redfish"},
	}

	id := req.AccountID

	if requestUser.UserName != "" {
		errorMessage := "error: username cannot be modified"
		resp.StatusCode = http.StatusBadRequest
		resp.StatusMessage = response.GeneralError
		args := response.Args{
			Code:    response.GeneralError,
			Message: errorMessage,
		}
		resp.Body = args.CreateGenericErrorResponse()
		resp.Header = map[string]string{
			"Content-type": "application/json; charset=utf-8", // TODO: add all error headers
		}
		log.Printf(errorMessage)
		return resp
	}

	if requestUser.RoleID != "" {
		if requestUser.RoleID != common.RoleAdmin {
			if requestUser.RoleID != common.RoleMonitor {
				if requestUser.RoleID != common.RoleClient {
					_, err := asmodel.GetRoleDetailsByID(requestUser.RoleID)
					if err != nil {
						errorMessage := "error: Invalid RoleID " + requestUser.RoleID + " present"
						resp.StatusCode = http.StatusBadRequest
						resp.StatusMessage = response.PropertyValueNotInList
						args := response.Args{
							Code:    response.GeneralError,
							Message: "",
							ErrorArgs: []response.ErrArgs{
								response.ErrArgs{
									StatusMessage: resp.StatusMessage,
									ErrorMessage:  errorMessage,
									MessageArgs:   []interface{}{requestUser.RoleID, "RoleID"},
								},
							},
						}
						resp.Body = args.CreateGenericErrorResponse()
						resp.Header = map[string]string{
							"Content-type": "application/json; charset=utf-8", // TODO: add all error headers
						}
						log.Printf(errorMessage)
						return resp
					}
				}
			}
		}

	}

	user, gerr := asmodel.GetUserDetails(id)
	if gerr != nil {
		errorMessage := "error while trying to get  account: " + gerr.Error()
		if errors.DBKeyNotFound == gerr.ErrNo() {
			resp.StatusCode = http.StatusNotFound
			resp.StatusMessage = response.ResourceNotFound
			args := response.Args{
				Code:    response.GeneralError,
				Message: "",
				ErrorArgs: []response.ErrArgs{
					response.ErrArgs{
						StatusMessage: resp.StatusMessage,
						ErrorMessage:  errorMessage,
						MessageArgs:   []interface{}{"Account", id},
					},
				},
			}
			resp.Body = args.CreateGenericErrorResponse()
		} else {
			resp.CreateInternalErrorResponse(errorMessage)
		}
		resp.Header = map[string]string{
			"Content-type": "application/json; charset=utf-8", // TODO: add all error headers
		}
		log.Printf(errorMessage)
		return resp
	}

	if user.UserName != session.UserName && !session.Privileges[common.PrivilegeConfigureUsers] {
		errorMessage := "error: user does not have the privilege to update other accounts"
		resp.StatusCode = http.StatusForbidden
		resp.StatusMessage = response.InsufficientPrivilege
		args := response.Args{
			Code:    response.GeneralError,
			Message: "",
			ErrorArgs: []response.ErrArgs{
				response.ErrArgs{
					StatusMessage: resp.StatusMessage,
					ErrorMessage:  errorMessage,
					MessageArgs:   []interface{}{},
				},
			},
		}
		resp.Body = args.CreateGenericErrorResponse()
		resp.Header = map[string]string{
			"Content-type": "application/json; charset=utf-8", // TODO: add all error headers
		}
		log.Printf(errorMessage)
		return resp
	}

	//To be discussed
	// Check if the user trying to update RoleID, if so check if he has PrivilegeConfigureUsers Privilege,
	// if not return 403 forbidden.
	// Without PrivilegeConfigureUsers user is not allowed to update any user account roleID, including his own account roleID
	if requestUser.RoleID != "" {
		if !session.Privileges[common.PrivilegeConfigureUsers] {
			errorMessage := "error: user does not have the privilege to update any account role, including his own account"
			resp.StatusCode = http.StatusForbidden
			resp.StatusMessage = response.InsufficientPrivilege
			args := response.Args{
				Code:    response.GeneralError,
				Message: "",
				ErrorArgs: []response.ErrArgs{
					response.ErrArgs{
						StatusMessage: resp.StatusMessage,
						ErrorMessage:  errorMessage,
						MessageArgs:   []interface{}{},
					},
				},
			}
			resp.Body = args.CreateGenericErrorResponse()
			resp.Header = map[string]string{
				"Content-type": "application/json; charset=utf-8", // TODO: add all error headers
			}
			log.Printf(errorMessage)
			return resp
		}
	}

	if requestUser.Password != "" {
		// Password modification not allowed, if user doesn't have ConfigureSelf or ConfigureUsers privilege
		if !session.Privileges[common.PrivilegeConfigureSelf] && !session.Privileges[common.PrivilegeConfigureUsers] {
			errorMessage := "error: roles, user is associated with, doesn't allow changing own or other users password"
			resp.StatusCode = http.StatusForbidden
			resp.StatusMessage = response.InsufficientPrivilege
			args := response.Args{
				Code:    response.GeneralError,
				Message: "",
				ErrorArgs: []response.ErrArgs{
					response.ErrArgs{
						StatusMessage: resp.StatusMessage,
						ErrorMessage:  errorMessage,
						MessageArgs:   []interface{}{},
					},
				},
			}
			resp.Body = args.CreateGenericErrorResponse()
			resp.Header = map[string]string{
				"Content-type": "application/json; charset=utf-8", // TODO: add all error headers
			}
			log.Printf(errorMessage)
			return resp
		}

		//TODO: handle all the combination of patch roles(admin,non-admin,default admin, non-default admin)
		if err = validatePassword(user.UserName, requestUser.Password); err != nil {
			errorMessage := err.Error()
			resp.StatusCode = http.StatusBadRequest
			resp.StatusMessage = response.PropertyValueFormatError
			args := response.Args{
				Code:    response.GeneralError,
				Message: "",
				ErrorArgs: []response.ErrArgs{
					response.ErrArgs{
						StatusMessage: resp.StatusMessage,
						ErrorMessage:  errorMessage,
						MessageArgs:   []interface{}{requestUser.Password, "Password"},
					},
				},
			}
			resp.Body = args.CreateGenericErrorResponse()
			resp.Header = map[string]string{
				"Content-type": "application/json; charset=utf-8", // TODO: add all error headers
			}
			log.Println(errorMessage)
			return resp
		}
		hash := sha3.New512()
		hash.Write([]byte(requestUser.Password))
		hashSum := hash.Sum(nil)
		hashedPassword := base64.URLEncoding.EncodeToString(hashSum)
		requestUser.Password = hashedPassword
	}

	if uerr := user.UpdateUserDetails(requestUser); uerr != nil {
		errorMessage := "error while trying to update user: " + uerr.Error()
		resp.CreateInternalErrorResponse(errorMessage)
		resp.Header = map[string]string{
			"Content-type": "application/json; charset=utf-8", // TODO: add all error headers
		}
		log.Printf(errorMessage)
		return resp
	}

	resp.StatusCode = http.StatusOK
	resp.StatusMessage = response.AccountModified

	resp.Header = map[string]string{
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"Content-type":      "application/json; charset=utf-8",
		"Link":              "</redfish/v1/AccountService/Accounts/" + user.UserName + "/>; rel=describedby",
		"Location":          "/redfish/v1/AccountService/Accounts/" + user.UserName + "/",
		"Transfer-Encoding": "chunked",
		"OData-Version":     "4.0",
	}

	commonResponse.CreateGenericResponse(resp.StatusMessage)
	resp.Body = asresponse.Account{
		Response:     commonResponse,
		UserName:     user.UserName,
		RoleID:       user.RoleID,
		AccountTypes: user.AccountTypes,
		Links: asresponse.Links{
			Role: asresponse.Role{
				OdataID: "/redfish/v1/AccountService/Roles/" + user.RoleID + "/",
			},
		},
	}

	return resp
}
