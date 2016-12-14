package main

import (
	"fmt"
	// "os/exec"
	"regexp"
	// "strings"
	// log "gopkg.in/inconshreveable/log15.v2"
	"github.com/lxc/lxd/shared"
)

func storagePoolsGetPools(d *Daemon) ([]string, error) {
	storagePools, err := dbStoragePools(d.db)
	if err != nil {
		return nil, err
	}

	return storagePools, nil
}

func storagePoolValidName(value string) error {
	// Validate the character set
	match, _ := regexp.MatchString("^[-a-zA-Z0-9]*$", value)
	if !match {
		return fmt.Errorf("Interface name contains invalid characters")
	}

	return nil
}

func doStoragePoolGet(d *Daemon, name string) (shared.StoragePoolConfig, error) {
	// Prepare the response
	s := shared.StoragePoolConfig{}
	s.Name = name
	s.UsedBy = []string{}
	s.Config = map[string]string{}

	_, pool, _ := dbStoragePoolGet(d.db, name)

	s.Name = pool.Name
	s.Driver = pool.Driver
	s.Config = pool.Config

	// Look for containers using this storage pool.
	// cts, err := dbContainersList(d.db, cTypeRegular)
	// if err != nil {
	// 	return shared.StoragePoolConfig{}, err
	// }

	// for _, ct := range cts {
	// 	c, err := containerLoadByName(d, ct)
	// 	if err != nil {
	// 		return shared.StoragePoolConfig{}, err
	// 	}
	// }

	return s, nil
}
