package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/lxc/lxd/shared"
)

// /1.0/storage-pools/<pool-name>/volumes
func storageVolumesGet(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumesPost(d *Daemon, r *http.Request) Response {
	req := shared.StorageVolumeConfig{}

	// Parse the request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return BadRequest(err)
	}

	// Sanity checks
	if req.PoolName == "" {
		return BadRequest(fmt.Errorf("No name provided"))
	}

	err = storageValidName(req.PoolName)
	if err != nil {
		return BadRequest(err)
	}

	// Check that the storage pool does not already exist.
	volumes, err := dbStorageVolumes(d.db)
	if err != nil {
		return InternalError(err)
	}

	if shared.StringInSlice(req.PoolName, volumes) {
		return Conflict
	}

	// Make sure that we don't pass a nil to the next function.
	if req.Config == nil {
		req.Config = map[string]string{}
	}

	pool, err := storageVolumeGetPoolName(r.URL.RequestURI())
	if err != nil {
		return InternalError(err)
	}

	_, poolStruct, err := dbStoragePoolGet(d.db, pool)
	if err != nil {
		return InternalError(err)
	}


	// Validate the requested storage pool configuration.
	err = storageVolumeValidateConfig(req.PoolName, req.Config, poolStruct)
	if err != nil {
		return BadRequest(err)
	}

	// Create the database entry for the storage pool.
	_, err = dbStorageVolumeCreate(d.db, req.PoolName, req.Config)
	if err != nil {
		return InternalError(fmt.Errorf("Error inserting %s into database: %s", req.PoolName, err))
	}

	// // Load the storage pool from the database.
	// s, err  := storageVolumeLoadByName(d, req.PoolName)
	// if err != nil {
	// 	return InternalError(err)
	// }

	// shared.LogErrorf("AAAA: %v", s.config)


	// Create storage pool.
	// err = s.storageVolumeCreate()
	// if err != nil {
	// 	// s.storagePoolDelete()
	// 	return InternalError(err)
	// }

	return SyncResponseLocation(true, nil, fmt.Sprintf("/%s/storage-pools/%s/volumes", shared.APIVersion, req.PoolName))
}

var storageVolumesCmd = Command{name: "storage-pools/{pool_name}/volumes", get: storageVolumesGet, post: storageVolumesPost}

// /1.0/storage-pools/<pool-name>/volumes/volume name>
func storageVolumeGet(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumePost(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumePut(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumePatch(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumeDelete(d *Daemon, r *http.Request) Response {
	return nil
}

var storageVolumeCmd = Command{name: "storage-pools/{pool}/volumes/{volume}", get: storageVolumeGet, post: storageVolumePost, put: storageVolumePut, patch: storageVolumePatch, delete: storageVolumeDelete}

// The storage pool structs and functions
type storageVolume struct {
	// Properties
	daemon *Daemon
	id     int64
	name   string

	// config
	config map[string]string
}

func storageVolumeLoadByName(d *Daemon, name string) (*storageVolume, error) {
	id, dbInfo, err := dbStorageVolumeGet(d.db, name)
	if err != nil {
		return nil, err
	}

	s := storageVolume{daemon: d, id: id, name: name, config: dbInfo.Config}

	return &s, nil
}

// func (s *storagePool) storageVolumeCreate() error {
// 	size := ""
// 
// 	// TODO: Again, we need to retrieve the driver of the parent pool.
// 	if s.config["driver"] != "dir" {
// 		if s.config["size"] == "" {
// 			st := syscall.Statfs_t{}
// 			err := syscall.Statfs(shared.VarPath(), &st)
// 			if err != nil {
// 				return fmt.Errorf("couldn't statfs %s: %s", shared.VarPath(), err)
// 			}
// 
// 			/* choose 15 GB < x < 100GB, where x is 20% of the disk size */
// 			sz := uint64(st.Frsize) * st.Blocks / (1024 * 1024 * 1024) / 5
// 			if sz > 100 {
// 				sz = 100
// 			}
// 			if sz < 15 {
// 				sz = 15
// 			}
// 
// 			size = strconv.FormatUint(sz, 10) + "GB"
// 			s.config["size"] = size
// 		}
// 
// 		if err := dbStoragePoolUpdate(s.daemon.db, s.name, s.config); err != nil {
// 			return fmt.Errorf("Failed to update database")
// 		}
// 	}
// 
// 	if s.config["driver"] == "zfs" {
// 		return s.zfsPoolCreate()
// 	}
// 
// 	return nil
// }
