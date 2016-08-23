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

package server

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	. "github.com/pingcap/check"
)

func TestServer(t *testing.T) {
	TestingT(t)
}

var stripUnix = strings.NewReplacer("unix://", "")

type cleanupFunc func()

func newTestServer(c *C) (*Server, cleanUpFunc) {
	cfg := NewTestSingleConfig()

	svr, err := NewServer(cfg)
	c.Assert(err, IsNil)

	cleanup := func() {
		svr.Close()
		os.RemoveAll(svr.cfg.DataDir)

		os.Remove(stripUnix.Replace(svr.cfg.PeerUrls))
		os.Remove(stripUnix.Replace(svr.cfg.ClientUrls))
		os.Remove(stripUnix.Replace(svr.cfg.AdvertisePeerUrls))
		os.Remove(stripUnix.Replace(svr.cfg.AdvertiseClientUrls))
	}

	return svr, cleanup
}

func newMultiTestServers(c *C, count int) ([]*Server, cleanupFunc) {
	svrs := make([]*Server, 0, count)
	cfgs := NewTestMultiConfig(count)
	urls := make([]string, 0, count*4)

	ch := make(chan *Server, count)
	for i := 0; i < count; i++ {
		cfg := cfgs[i]

		urls = append(urls, cfg.PeerUrls)
		urls = append(urls, cfg.ClientUrls)
		urls = append(urls, cfg.AdvertiseClientUrls)
		urls = append(urls, cfg.AdvertisePeerUrls)

		go func() {
			svr, err := NewServer(cfg)
			c.Assert(err, IsNil)
			ch <- svr
		}()
	}

	for i := 0; i < count; i++ {
		svr := <-ch
		go svr.Run()
		svrs = append(svrs, svr)
	}

	mustWaitLeader(c, svrs)

	cleanup := func() {
		for _, svr := range svrs {
			svr.Close()
			os.RemoveAll(svr.cfg.DataDir)
		}
		for _, u := range urls {
			os.Remove(stripUnix.Replace(u))
		}
	}

	return svrs, cleanup
}

func mustWaitLeader(c *C, svrs []*Server) *Server {
	for i := 0; i < 100; i++ {
		for _, s := range svrs {
			if s.IsLeader() {
				return s
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.Fatal("no leader")
	return nil
}

var _ = Suite(&testLeaderServerSuite{})

type testLeaderServerSuite struct {
	client     *clientv3.Client
	svrs       map[string]*Server
	leaderPath string
}

func (s *testLeaderServerSuite) SetUpSuite(c *C) {
	s.svrs = make(map[string]*Server)

	cfgs := NewTestMultiConfig(3)

	ch := make(chan *Server, 3)
	for i := 0; i < 3; i++ {
		cfg := cfgs[i]

		go func() {
			svr, err := NewServer(cfg)
			c.Assert(err, IsNil)
			ch <- svr
		}()
	}

	for i := 0; i < 3; i++ {
		svr := <-ch
		s.svrs[svr.GetAddr()] = svr
		s.leaderPath = svr.getLeaderPath()
	}

	s.setUpClient(c)
}

func (s *testLeaderServerSuite) TearDownSuite(c *C) {
	for _, svr := range s.svrs {
		svr.Close()
	}
	s.client.Close()

	// Make sure unix socket is removed.
	for _, svr := range s.svrs {
		os.Remove(stripUnix.Replace(svr.cfg.PeerUrls))
		os.Remove(stripUnix.Replace(svr.cfg.ClientUrls))
		os.Remove(stripUnix.Replace(svr.cfg.AdvertisePeerUrls))
		os.Remove(stripUnix.Replace(svr.cfg.AdvertiseClientUrls))
	}
}

func (s *testLeaderServerSuite) setUpClient(c *C) {
	endpoints := make([]string, 0, 3)

	for _, svr := range s.svrs {
		endpoints = append(endpoints, svr.GetEndpoints()...)
	}

	var err error
	s.client, err = clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 3 * time.Second,
	})
	c.Assert(err, IsNil)
}

func (s *testLeaderServerSuite) TestLeader(c *C) {
	for _, svr := range s.svrs {
		go svr.Run()
	}

	leader1 := mustGetLeader(c, s.client, s.leaderPath)
	svr, ok := s.svrs[leader1.GetAddr()]
	c.Assert(ok, IsTrue)
	svr.Close()
	delete(s.svrs, leader1.GetAddr())

	// wait leader changes
	for i := 0; i < 50; i++ {
		leader, _ := getLeader(s.client, s.leaderPath)
		if leader != nil && leader.GetAddr() != leader1.GetAddr() {
			break
		}

		time.Sleep(500 * time.Millisecond)
	}

	leader2 := mustGetLeader(c, s.client, s.leaderPath)
	c.Assert(leader1.GetAddr(), Not(Equals), leader2.GetAddr())
}
