package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
)

type containerPostBody struct {
	Migration bool   `json:"migration"`
	Mode      string `json:"mode"`
	Name      string `json:"name"`
	Live      bool   `json:"live"`
}

func containerPost(d *Daemon, r *http.Request) Response {
	var (
		name string
		c    container
		err  error
	)

	name = mux.Vars(r)["name"]

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return InternalError(err)
	}

	body := containerPostBody{}
	if err := json.Unmarshal(buf, &body); err != nil {
		return BadRequest(err)
	}

	if body.Mode == "pull" {
		c, err = containerLoadByName(d, name)
		if err != nil {
			return SmartError(err)
		}
	}

	if body.Migration {
		ws, err := NewMigrationSource(c)
		if err != nil {
			return InternalError(err)
		}

		resources := map[string][]string{}
		resources["containers"] = []string{name}

		op, err := operationCreate(operationClassWebsocket, resources, ws.Metadata(), ws.Do, nil, ws.Connect)
		if err != nil {
			return InternalError(err)
		}

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
