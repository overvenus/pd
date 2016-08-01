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
	"net/http"
	"time"

	"github.com/pingcap/pd/server"
	"github.com/unrolled/render"
	"golang.org/x/net/context"
)

const defaultDialTimeout = 5 * time.Second

type memberInfo struct {
	Name       string   `json:"name"`
	ClientURLs []string `json:"client-ursl"`
	PeerURLs   []string `json:"peer-urls"`
}

type memberListHandler struct {
	svr *server.Server
	rd  *render.Render
}

func newMemberListHandler(svr *server.Server, rd *render.Render) *memberListHandler {
	return &memberListHandler{
		svr: svr,
		rd:  rd,
	}
}

func (h *memberListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultDialTimeout)
	defer cancel()
	client := h.svr.GetClient()

	listResp, _ := client.MemberList(ctx)
	memberInfos := make([]memberInfo, 0, len(listResp.Members))
	for _, m := range listResp.Members {
		info := memberInfo{
			Name:       m.Name,
			ClientURLs: m.ClientURLs,
			PeerURLs:   m.PeerURLs,
		}
		memberInfos = append(memberInfos, info)
	}

	h.rd.JSON(w, http.StatusOK, memberInfos)
}
