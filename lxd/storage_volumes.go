package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/lxc/lxd/shared"
	log "gopkg.in/inconshreveable/log15.v2"
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

	pool := req.PoolName
	if pool == "" {
		pool, err = storageVolumeGetPoolName(r.URL.RequestURI())
		if err != nil {
			return InternalError(err)
		}
	}

	_, poolStruct, err := dbStoragePoolGet(d.db, pool)
	if err != nil {
		return InternalError(err)
	}

	// Validate the requested storage pool configuration.
	err = storageVolumeValidateConfig(pool, req.Config, poolStruct)
	if err != nil {
		return BadRequest(err)
	}

	// Create the database entry for the storage pool.
	_, err = dbStorageVolumeCreate(d.db, req.VolumeName, req.Config)
	if err != nil {
		return InternalError(fmt.Errorf("Error inserting %s into database: %s", pool, err))
	}

	// Load the storage pool from the database.
	s, err := storageVolumeLoadByName(d, req.VolumeName, "")
	if err != nil {
		return InternalError(err)
	}

	s.parentPool = poolStruct

	// Create storage pool.
	err = s.storageVolumeCreate()
	if err != nil {
		// s.storagePoolDelete()
		return InternalError(err)
	}

	return SyncResponseLocation(true, nil, fmt.Sprintf("/%s/storage-pools/%s/volumes", shared.APIVersion, pool))
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

	volumeName   string
	volumeConfig map[string]string

	parentPool *shared.StoragePoolConfig
}

func storageVolumeLoadByName(d *Daemon, volumeName string, poolName string) (*storageVolume, error) {
	id, volume, err := dbStorageVolumeGet(d.db, volumeName)
	if err != nil {
		return nil, err
	}

	s := storageVolume{daemon: d, id: id, volumeName: volumeName, volumeConfig: volume.Config}

	parentPool := &shared.StoragePoolConfig{}
	if poolName != "" {
		_, s.parentPool, err = dbStoragePoolGet(d.db, poolName)
		if err != nil {
			return nil, err
		}
		s.parentPool = parentPool
	}

	return &s, nil
}

func (s *storageVolume) storageVolumeCreate() error {
	size := ""

	// TODO: Again, we need to retrieve the driver of the parent pool.
	driver := s.parentPool.Config["driver"]
	if driver != "dir" {
		if s.volumeConfig["size"] == "" {
			st := syscall.Statfs_t{}
			err := syscall.Statfs(shared.VarPath(), &st)
			if err != nil {
				return fmt.Errorf("couldn't statfs %s: %s", shared.VarPath(), err)
			}

			/* choose 15 GB < x < 100GB, where x is 20% of the disk size */
			sz := uint64(st.Frsize) * st.Blocks / (1024 * 1024 * 1024) / 5
			if sz > 100 {
				sz = 100
			}
			if sz < 15 {
				sz = 15
			}

			size = strconv.FormatUint(sz, 10) + "GB"
			s.volumeConfig["size"] = size
		}

		if err := dbStorageVolumeUpdate(s.daemon.db, s.volumeName, s.volumeConfig); err != nil {
			return fmt.Errorf("Failed to update database")
		}
	}

	if driver == "zfs" {
		return s.zfsVolumeCreate()
	}

	return nil
}

func (s *storageVolume) zfsVolumeCreate() error {
	out, err := exec.LookPath("zfs")
	if err != nil || len(out) == 0 {
		return fmt.Errorf("The 'zfs' tool isn't available")
	}

	zpoolName := s.parentPool.Config["zfs.pool_name"]
	if zpoolName == "" {
		zpoolName = s.parentPool.PoolName
	}

	size := s.volumeConfig["size"]
	if size == "" {
		size = s.parentPool.Config["volume.size"]
		if size == "" {
			return fmt.Errorf("A size parameter is required.")
		}
	}

	output, err := exec.Command(
		"zfs",
		"create",
		"-p",
		fmt.Sprintf("%s/%s", zpoolName, s.volumeName)).CombinedOutput()
	if err != nil {
		shared.LogErrorf("zfs create failed", log.Ctx{"output": string(output)})
		return fmt.Errorf("Failed to create ZFS filesystem: %s", output)
	}

	return nil
}
