// Copyright 2018 Google Cloud Platform Proxy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package integration

import (
	"fmt"
	"testing"
	"time"

	"cloudesf.googlesource.com/gcpproxy/src/go/util"
	"cloudesf.googlesource.com/gcpproxy/tests/endpoints/echo/client"
	"cloudesf.googlesource.com/gcpproxy/tests/env"
	"cloudesf.googlesource.com/gcpproxy/tests/utils"

	comp "cloudesf.googlesource.com/gcpproxy/tests/env/components"
)

func TestDynamicRouting(t *testing.T) {
	serviceName := "test-echo"
	configId := "test-config-id"

	args := []string{"--service=" + serviceName, "--version=" + configId,
		"--backend_protocol=http1", "--rollout_strategy=fixed", "--enable_backend_routing"}

	s := env.NewTestEnv(comp.TestDynamicRouting, "echo", nil)
	s.EnableDynamicRoutingBackend()
	if err := s.Setup(args); err != nil {
		t.Fatalf("fail to setup test env, %v", err)
	}
	defer s.TearDown()

	testData := []struct {
		desc          string
		path          string
		method        string
		message       string
		wantResp      string
		httpCallError error
	}{
		{
			desc:     "Succeed, no path translation (no re-routing needed)",
			path:     "/echo?key=api-key",
			method:   "POST",
			message:  "hello",
			wantResp: `{"message":"hello"}`,
		},
		{
			desc:     "Succeed, CONSTANT_ADDRESS path translation is correct",
			path:     "/pet/123/num/987",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/getpetbyid","RawQuery":"pet_id=123&number=987"}`,
		},
		{
			desc:     "Succeed, CONSTANT_ADDRESS path translation is correct, original URL has query parameters, original query parameters should appear first and query parameters converted from path parameters appear later",
			path:     "/pet/31/num/565?lang=US&zone=us-west1",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/getpetbyid","RawQuery":"lang=US&zone=us-west1&pet_id=31&number=565"}`,
		},
		{
			desc:     "Succeed, CONSTANT_ADDRESS path translation is correct, original URL has query parameters, original query parameters should appear first and query parameters converted from path parameters appear later",
			path:     "/pet/31/num/565?lang=US&zone=us-west1",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/getpetbyid","RawQuery":"lang=US&zone=us-west1&pet_id=31&number=565"}`,
		},
		{
			desc:     "Succeed, CONSTANT_ADDRESS path translation with snake case is correct",
			path:     "/shelves/123/books/info/987",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/bookinfo","RawQuery":"SHELF=123&BOOK=987"}`,
		},
		{
			desc:     "Succeed, CONSTANT_ADDRESS path translation with snake case is correct, supports {foo.bar} style path, if corresponding jsonName not found, origin snake case path is used.",
			path:     "/shelves/221/books/id/2019",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/bookid","RawQuery":"SHELF.i_d=221&BOOK.id=2019"}`,
		},
		{
			desc:     "Succeed, APPEND_PATH_TO_ADDRESS path translation is correct, appends original URL to backend address (https://domain/base/path)",
			path:     "/searchpet",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/searchpet/searchpet","RawQuery":""}`,
		},
		{
			desc:     "Succeed, APPEND_PATH_TO_ADDRESS path translation with query parameter is correct, appends original URL to backend address (https://domain/base/path)",
			path:     "/searchpet?timezone=PST&lang=US",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/searchpet/searchpet","RawQuery":"timezone=PST&lang=US"}`,
		},
		{
			desc:     "Succeed, APPEND_PATH_TO_ADDRESS path translation is correct, appends original URL to backend address that ends with slash (https://domain/base/path/)",
			path:     "/searchdog",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/searchdogs/searchdog","RawQuery":""}`,
		},
		{
			desc:     "Succeed, APPEND_PATH_TO_ADDRESS path translation with query parameter is correct, appends original URL to backend address that ends with slash (https://domain/base/path/)",
			path:     "/searchdog?timezone=UTC",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/searchdogs/searchdog","RawQuery":"timezone=UTC"}`,
		},
		{
			desc:     "Succeed, APPEND_PATH_TO_ADDRESS path translation is correct, original URL has path parameters",
			path:     "/pets/cat/year/2018",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/listpet/pets/cat/year/2018","RawQuery":""}`,
		},
		{
			desc:     "Succeed, APPEND_PATH_TO_ADDRESS path translation is correct, original URL has path parameters and query parameters",
			path:     "/pets/dog/year/2019?lang=US&zone=us-west1",
			method:   "GET",
			wantResp: `{"Path":"/dynamicrouting/listpet/pets/dog/year/2019","RawQuery":"lang=US&zone=us-west1"}`,
		},
		{
			desc:     "Succeed, APPEND_PATH_TO_ADDRESS path translation is correct, backend address is root path with slash (https://domain/)",
			path:     "/searchrootwithslash",
			method:   "GET",
			wantResp: `{"Path":"/searchrootwithslash","RawQuery":""}`,
		},
		{
			desc:     "Succeed, APPEND_PATH_TO_ADDRESS path translation with query parameter is correct, backend address is root path with slash (https://domain/)",
			path:     "/searchroot?zone=us-central1&lang=en",
			method:   "GET",
			wantResp: `{"Path":"/searchroot","RawQuery":"zone=us-central1&lang=en"}`,
		},
		{
			desc:          "Fail, there is not backend rule specified for this path",
			path:          "/searchdogs",
			method:        "GET",
			httpCallError: fmt.Errorf("http response status is not 200 OK: 404 Not Found"),
		},
	}
	for _, tc := range testData {
		url := fmt.Sprintf("http://localhost:%v%v", s.Ports().ListenerPort, tc.path)
		var gotResp []byte
		var err error
		if tc.method == "GET" {
			gotResp, err = client.DoGet(url)

		} else if tc.method == "POST" {
			gotResp, err = client.DoPost(url, tc.message)
		} else {
			t.Fatalf("unknown HTTP method (%v) to call", tc.method)
		}

		if tc.httpCallError == nil {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			if tc.httpCallError.Error() != err.Error() {
				t.Errorf("expected Http call error: %v, got: %v", tc.httpCallError, err)
			}
			continue
		}
		gotRespStr := utils.NormalizeJson(string(gotResp))

		if gotRespStr != utils.NormalizeJson(tc.wantResp) {
			t.Errorf("response expected: %s, got: %s", tc.wantResp, gotRespStr)
		}
	}
}

func TestServiceControlRequestForDynamicRouting(t *testing.T) {
	serviceName := "test-echo"
	configId := "test-config-id"

	args := []string{"--service=" + serviceName, "--version=" + configId,
		"--backend_protocol=http1", "--rollout_strategy=fixed", "--enable_backend_routing"}

	s := env.NewTestEnv(comp.TestServiceControlRequestInDynamicRouting, "echo", nil)
	s.EnableDynamicRoutingBackend()
	if err := s.Setup(args); err != nil {
		t.Fatalf("fail to setup test env, %v", err)
	}
	defer s.TearDown()

	testData := []struct {
		desc           string
		path           string
		method         string
		message        string
		wantResp       string
		wantScRequests []interface{}
	}{
		{
			desc:     "Succeed, no path translation (no re-routing needed)",
			path:     "/echo?key=api-key",
			message:  "hello",
			wantResp: `{"message":"hello"}`,
			wantScRequests: []interface{}{
				&utils.ExpectedCheck{
					Version:         utils.APIProxyVersion,
					ServiceName:     "echo-api.endpoints.cloudesf-testing.cloud.goog",
					ServiceConfigID: "test-config-id",
					ConsumerID:      "api_key:api-key",
					OperationName:   "1.echo_api_endpoints_cloudesf_testing_cloud_goog.Echo",
					CallerIp:        "127.0.0.1",
				},
				&utils.ExpectedReport{
					Version:           utils.APIProxyVersion,
					ServiceName:       "echo-api.endpoints.cloudesf-testing.cloud.goog",
					ServiceConfigID:   "test-config-id",
					URL:               "/echo?key=api-key",
					ApiKey:            "api-key",
					ApiMethod:         "1.echo_api_endpoints_cloudesf_testing_cloud_goog.Echo",
					ProducerProjectID: "producer-project",
					ConsumerProjectID: "123456",
					HttpMethod:        "POST",
					LogMessage:        "1.echo_api_endpoints_cloudesf_testing_cloud_goog.Echo is called",
					RequestSize:       20,
					ResponseSize:      19,
					RequestBytes:      20,
					ResponseBytes:     19,
					ResponseCode:      200,
					Platform:          util.GCE,
					Location:          "test-zone",
				},
			},
		},
		{
			desc:     "Succeed, APPEND_PATH_TO_ADDRESS path translation is correct, service control check request and report request are correct",
			path:     "/sc/searchpet?key=api-key&timezone=EST",
			message:  "hello",
			wantResp: `{"Path":"/dynamicrouting/sc/searchpet","RawQuery":"key=api-key&timezone=EST"}`,
			wantScRequests: []interface{}{
				&utils.ExpectedCheck{
					Version:         utils.APIProxyVersion,
					ServiceName:     "echo-api.endpoints.cloudesf-testing.cloud.goog",
					ServiceConfigID: "test-config-id",
					ConsumerID:      "api_key:api-key",
					OperationName:   "1.echo_api_endpoints_cloudesf_testing_cloud_goog.dynamic_routing.SearchPetWithServiceControlVerification",
					CallerIp:        "127.0.0.1",
				},
				&utils.ExpectedReport{
					Version:           utils.APIProxyVersion,
					ServiceName:       "echo-api.endpoints.cloudesf-testing.cloud.goog",
					ServiceConfigID:   "test-config-id",
					URL:               "/sc/searchpet?key=api-key&timezone=EST",
					ApiKey:            "api-key",
					ApiMethod:         "1.echo_api_endpoints_cloudesf_testing_cloud_goog.dynamic_routing.SearchPetWithServiceControlVerification",
					ProducerProjectID: "producer-project",
					ConsumerProjectID: "123456",
					HttpMethod:        "POST",
					LogMessage:        "1.echo_api_endpoints_cloudesf_testing_cloud_goog.dynamic_routing.SearchPetWithServiceControlVerification is called",
					RequestSize:       20,
					ResponseSize:      82,
					RequestBytes:      20,
					ResponseBytes:     82,
					ResponseCode:      200,
					Platform:          util.GCE,
					Location:          "test-zone",
				},
			},
		},
		{
			desc:     "Succeed, CONSTANT_ADDRESS path translation is correct, service control check request and report request are correct",
			path:     "/sc/pet/0325/num/2019?key=api-key&lang=en",
			message:  "hello",
			wantResp: `{"Path":"/dynamicrouting","RawQuery":"key=api-key&lang=en&pet_id=0325&number=2019"}`,
			wantScRequests: []interface{}{
				&utils.ExpectedCheck{
					Version:         utils.APIProxyVersion,
					ServiceName:     "echo-api.endpoints.cloudesf-testing.cloud.goog",
					ServiceConfigID: "test-config-id",
					ConsumerID:      "api_key:api-key",
					OperationName:   "1.echo_api_endpoints_cloudesf_testing_cloud_goog.dynamic_routing.GetPetByIdWithServiceControlVerification",
					CallerIp:        "127.0.0.1",
				},
				&utils.ExpectedReport{
					Version:           utils.APIProxyVersion,
					ServiceName:       "echo-api.endpoints.cloudesf-testing.cloud.goog",
					ServiceConfigID:   "test-config-id",
					URL:               "/sc/pet/0325/num/2019?key=api-key&lang=en",
					ApiKey:            "api-key",
					ApiMethod:         "1.echo_api_endpoints_cloudesf_testing_cloud_goog.dynamic_routing.GetPetByIdWithServiceControlVerification",
					ProducerProjectID: "producer-project",
					ConsumerProjectID: "123456",
					HttpMethod:        "POST",
					LogMessage:        "1.echo_api_endpoints_cloudesf_testing_cloud_goog.dynamic_routing.GetPetByIdWithServiceControlVerification is called",
					RequestSize:       20,
					ResponseSize:      98,
					RequestBytes:      20,
					ResponseBytes:     98,
					ResponseCode:      200,
					Platform:          util.GCE,
					Location:          "test-zone",
				},
			},
		},
	}
	for _, tc := range testData {
		url := fmt.Sprintf("http://localhost:%v%v", s.Ports().ListenerPort, tc.path)
		var gotResp []byte
		var err error
		gotResp, err = client.DoPost(url, tc.message)

		if err != nil {
			t.Fatal(err)
		}

		gotRespStr := utils.NormalizeJson(string(gotResp))

		if gotRespStr != utils.NormalizeJson(tc.wantResp) {
			t.Errorf("Test Desc(%s): response expected: %s, got: %s", tc.desc, tc.wantResp, gotRespStr)
		}
		scRequests, err1 := s.ServiceControlServer.GetRequests(len(tc.wantScRequests), 2*time.Second)
		if err1 != nil {
			t.Fatalf("Test Desc(%s): GetRequests returns error: %v", tc.desc, err1)
		}

		for i, wantScRequest := range tc.wantScRequests {
			reqBody := scRequests[i].ReqBody
			switch wantScRequest.(type) {
			case *utils.ExpectedCheck:
				if scRequests[i].ReqType != comp.CHECK_REQUEST {
					t.Errorf("Test Desc(%s): service control request %v: should be Check", tc.desc, i)
				}
				if err := utils.VerifyCheck(reqBody, wantScRequest.(*utils.ExpectedCheck)); err != nil {
					t.Error(err)
				}
			case *utils.ExpectedReport:
				if scRequests[i].ReqType != comp.REPORT_REQUEST {
					t.Errorf("Test Desc(%s): service control request %v: should be Report", tc.desc, i)
				}
				if err := utils.VerifyReport(reqBody, wantScRequest.(*utils.ExpectedReport)); err != nil {
					t.Error(err)
				}
			default:
				t.Fatalf("Test Desc(%s): unknown service control response type", tc.desc)
			}
		}
	}
}
