package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/lxc/lxd/shared"
)

type containerPostBody struct {
	Migration bool   `json:"migration"`
	Mode      string `json:"mode"`
	Name      string `json:"name"`
}

func containerPost(d *Daemon, r *http.Request) Response {
	var (
		name string
		mode string
		c    container
		err  error
	)

	mode = mux.Vars(r)["Mode"]
	if mode == "pull" {
		name = mux.Vars(r)["name"]
		c, err = containerLoadByName(d, name)
		if err != nil {
			shared.LogWarnf("0000")
			return SmartError(err)
		}
		shared.LogWarnf("0000.1111")
	}
	shared.LogWarnf("1111")

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return InternalError(err)
	}
	shared.LogWarnf("2222")

	body := containerPostBody{}
	if err := json.Unmarshal(buf, &body); err != nil {
		return BadRequest(err)
	}
	shared.LogWarnf("3333")

	if body.Migration {
		ws, err := NewMigrationSource(c)
		if err != nil {
			return InternalError(err)
		}
		shared.LogWarnf("4444")

		resources := map[string][]string{}
		resources["containers"] = []string{name}

		op, err := operationCreate(operationClassWebsocket, resources, ws.Metadata(), ws.Do, nil, ws.Connect)
		if err != nil {
			return InternalError(err)
		}
		shared.LogWarnf("5555")

		return OperationResponse(op)
	}

	// Check that the name isn't already in use
	id, _ := dbContainerId(d.db, body.Name)
	if id > 0 {
		return Conflict
	}

	run := func(*operation) error {
		return c.Rename(body.Name)
	}

	resources := map[string][]string{}
	resources["containers"] = []string{name}

	op, err := operationCreate(operationClassTask, resources, nil, run, nil, nil)
	if err != nil {
		return InternalError(err)
	}

	return OperationResponse(op)
}
