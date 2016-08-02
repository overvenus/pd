// Copyright 2016 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	. "github.com/pingcap/check"
	"github.com/pingcap/pd/server"
)

func TestJoin(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&testMemberAPISuite{})

type testMemberAPISuite struct {
	hc *http.Client
}

func (s *testMemberAPISuite) SetUpSuite(c *C) {
	s.hc = mustNewHTTPClient()
}

func mustNewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
	}
}

type CleanUpFunc func()

func mustNewCluster(c *C, num int) ([]*server.Config, []*server.Server, CleanUpFunc) {
	dirs := make([]string, 0, num)
	svrs := make([]*server.Server, 0, num)
	cfgs := server.NewTestMultiConfig(num)

	ch := make(chan *server.Server, num)
	for _, cfg := range cfgs {
		dirs = append(dirs, cfg.DataDir)

		go func(cfg *server.Config) {
			s, e := server.NewServer(cfg)
			c.Assert(e, IsNil)
			go s.Run()
			go ServeHTTP(cfg.HTTPAddr, s)
			ch <- s
		}(cfg)
	}

	for i := 0; i < num; i++ {
		svr := <-ch
		svrs = append(svrs, svr)
	}
	close(ch)

	// wait etcds and http servers
	time.Sleep(5 * time.Second)

	// clean up
	return cfgs, svrs, func() {
		for _, s := range svrs {
			s.Close()
		}
		for _, dir := range dirs {
			os.RemoveAll(dir)
		}
	}
}

func checkListResponse(c *C, body []byte, cfgs []*server.Config) {
	got := make(map[string][]memberInfo)
	json.Unmarshal(body, &got)

	c.Assert(len(got["members"]), Equals, len(cfgs))

	for _, memb := range got["members"] {
		ok := false
		for _, cfg := range cfgs {
			if memb.Name == cfg.Name {
				mClientUrls := memb.ClientUrls
				sort.Strings(mClientUrls)
				stringOfmClientUrls := strings.Join(mClientUrls, ",")

				cClientUrls := strings.Split(cfg.ClientUrls, ",")
				sort.Strings(cClientUrls)
				stringOfcClientUrls := strings.Join(cClientUrls, ",")

				if stringOfmClientUrls == stringOfcClientUrls {
					mPeerUrls := memb.PeerUrls
					sort.Strings(mPeerUrls)
					stringOfmPeerUrls := strings.Join(mPeerUrls, ",")

					cPeerUrls := strings.Split(cfg.PeerUrls, ",")
					sort.Strings(cPeerUrls)
					stringOfcPeerUrls := strings.Join(cPeerUrls, ",")

					if stringOfmPeerUrls == stringOfcPeerUrls {
						ok = true
					}
				}
			}
		}
		c.Assert(ok, IsTrue)
	}
}

func (s *testMemberAPISuite) TestMemberList(c *C) {
	cfgs, _, clean := mustNewCluster(c, 1)
	defer clean()

	parts := []string{"http://", cfgs[0].HTTPAddr, "/api/v1/members"}
	resp, err := s.hc.Get(strings.Join(parts, ""))
	c.Assert(err, IsNil)
	buf, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	checkListResponse(c, buf, cfgs)

	cfgs, _, clean = mustNewCluster(c, 3)
	defer clean()

	parts = []string{"http://", cfgs[rand.Intn(len(cfgs))].HTTPAddr, "/api/v1/members"}
	resp, err = s.hc.Get(strings.Join(parts, ""))
	c.Assert(err, IsNil)
	buf, err = ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	checkListResponse(c, buf, cfgs)
}

func (s *testMemberAPISuite) TestMemberDelete(c *C) {
	cfgs, _, clean := mustNewCluster(c, 3)
	defer clean()

	foo := fmt.Sprintf("pd%d", rand.Int63())
	parts := []string{"http://", cfgs[rand.Intn(len(cfgs))].HTTPAddr, "/api/v1/members", foo}
	req, err := http.NewRequest(http.MethodDelete, strings.Join(parts, ""), nil)
	c.Assert(err, IsNil)
	resp, err := s.hc.Do(req)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusNotFound)

	someone := rand.Intn(len(cfgs))
	parts = []string{"http://", cfgs[rand.Intn(len(cfgs))].HTTPAddr, "/api/v1/members", "/" + cfgs[someone].Name}
	req, err = http.NewRequest(http.MethodDelete, strings.Join(parts, ""), nil)
	c.Assert(err, IsNil)
	resp, err = s.hc.Do(req)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Equals, http.StatusOK)

	// delete again
	req, err = http.NewRequest(http.MethodDelete, strings.Join(parts, ""), nil)
	c.Assert(err, IsNil)
	resp, err = s.hc.Do(req)
	c.Assert(err, IsNil)
	c.Assert(resp.StatusCode, Not(Equals), http.StatusOK)

	// delete someone
	newCfgs := append(cfgs[:someone], cfgs[someone+1:]...)
	parts = []string{"http://", cfgs[rand.Intn(len(newCfgs))].HTTPAddr, "/api/v1/members"}
	resp, err = s.hc.Get(strings.Join(parts, ""))
	c.Assert(err, IsNil)
	buf, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	checkListResponse(c, buf, newCfgs)
}
