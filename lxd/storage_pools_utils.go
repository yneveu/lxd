package main

import (
	// "os/exec"
	"os"
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

func doStoragePoolGet(d *Daemon, name string) (shared.StoragePoolConfig, error) {
	_, pool, _ := dbStoragePoolGet(d.db, name)

	// Sanity check
	if pool == nil {
		return shared.StoragePoolConfig{}, os.ErrNotExist
	}

	// Prepare the response
	s := shared.StoragePoolConfig{}
	s.Name = name
	s.UsedBy = []string{}
	s.Config = map[string]string{}

	s.Name = pool.Name
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

type sourceType int

const (
	sourcePath sourceType = iota
	sourceLoopfile
	sourceBlockDev
	sourceZfsDataset
)

func detectSourceType(source string) (sourceType, error) {
	return sourcePath, nil
}
