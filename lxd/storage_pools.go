package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/lxc/lxd/shared"
)

// /1.0/storage-pools
func storagePoolsGet(d *Daemon, r *http.Request) Response {
	recursionStr := r.FormValue("recursion")
	recursion, err := strconv.Atoi(recursionStr)
	if err != nil {
		recursion = 0
	}

	pools, err := storagePoolsGetPools(d)
	if err != nil {
		return InternalError(err)
	}

	resultString := []string{}
	resultMap := []shared.StoragePoolConfig{}
	for _, pool := range pools {
		if recursion == 0 {
			resultString = append(resultString, fmt.Sprintf("/%s/storage-pools/%s", shared.APIVersion, pool))
		} else {
			net, err := doStoragePoolGet(d, pool)
			if err != nil {
				continue
			}
			resultMap = append(resultMap, net)
		}
	}

	if recursion == 0 {
		return SyncResponse(true, resultString)
	}

	return SyncResponse(true, resultMap)
}

func storagePoolsPost(d *Daemon, r *http.Request) Response {
	req := shared.StoragePoolConfig{}

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
	pools, err := dbStoragePools(d.db)
	if err != nil {
		return InternalError(err)
	}

	if shared.StringInSlice(req.PoolName, pools) {
		return Conflict
	}

	// Make sure that we don't pass a nil to the next function.
	if req.Config == nil {
		req.Config = map[string]string{}
	}

	// Validate the requested storage pool configuration.
	err = storagePoolValidateConfig(req.PoolName, req.Config)
	if err != nil {
		return BadRequest(err)
	}

	// Create the database entry for the storage pool.
	_, err = dbStoragePoolCreate(d.db, req.PoolName, req.Config)
	if err != nil {
		return InternalError(
			fmt.Errorf("Error inserting %s into database: %s", req.PoolName, err))
	}

	// Load the storage pool from teh database.
	s, err := storagePoolLoadByName(d, req.PoolName)
	if err != nil {
		return InternalError(err)
	}

	// Create storage pool.
	err = s.storagePoolCreate()
	if err != nil {
		// s.storagePoolDelete()
		return InternalError(err)
	}

	return SyncResponseLocation(true, nil, fmt.Sprintf("/%s/storage-pools/%s", shared.APIVersion, req.PoolName))
}

var storagePoolsCmd = Command{name: "storage-pools", get: storagePoolsGet, post: storagePoolsPost}

// /1.0/storage-pools/<pool name>
func storagePoolGet(d *Daemon, r *http.Request) Response {
	name := mux.Vars(r)["pool_name"]

	s, err := doStoragePoolGet(d, name)
	if err != nil {
		return SmartError(err)
	}

	etag := []interface{}{s.PoolName, s.UsedBy, s.Config}

	return SyncResponseETag(true, &s, etag)
}

func storagePoolPost(d *Daemon, r *http.Request) Response {
	name := mux.Vars(r)["pool_name"]
	req := shared.StoragePoolConfig{}

	// Parse the request.
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return BadRequest(err)
	}

	// Get the existing storage pool.
	s, err := storagePoolLoadByName(d, name)
	if err != nil {
		return NotFound
	}

	// Sanity checks.
	if req.PoolName == "" {
		return BadRequest(fmt.Errorf("No name provided"))
	}

	err = storageValidName(req.PoolName)
	if err != nil {
		return BadRequest(err)
	}

	// Rename it
	err = s.storagePoolRename(req.PoolName)
	if err != nil {
		return SmartError(err)
	}

	return SyncResponseLocation(true, nil, fmt.Sprintf("/%s/networks/%s", shared.APIVersion, req.PoolName))
}

func storagePoolPut(d *Daemon, r *http.Request) Response {
	name := mux.Vars(r)["pool_name"]

	// Get the existing storage pool.
	_, dbInfo, err := dbStoragePoolGet(d.db, name)
	if err != nil {
		return SmartError(err)
	}

	// Validate the ETag
	etag := []interface{}{dbInfo.PoolName, dbInfo.UsedBy, dbInfo.Config}

	err = etagCheck(r, etag)
	if err != nil {
		return PreconditionFailed(err)
	}

	req := shared.StoragePoolConfig{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return BadRequest(err)
	}

	return doStoragePoolUpdate(d, name, dbInfo.Config, req.Config)
}

func storagePoolPatch(d *Daemon, r *http.Request) Response {
	return nil
}

func storagePoolDelete(d *Daemon, r *http.Request) Response {
	name := mux.Vars(r)["pool_name"]

	// Get the existing storage pool.
	s, err := storagePoolLoadByName(d, name)
	if err != nil {
		return NotFound
	}

	if s.config["driver"] == "zfs" {
		if err := s.zfsPoolDelete(); err != nil {
			return InternalError(err)
		}
	}

	if err := dbStoragePoolDelete(d.db, name); err != nil {
		return InternalError(err)
	}

	return EmptySyncResponse
}

var storagePoolCmd = Command{name: "storage-pools/{pool_name}", get: storagePoolGet, post: storagePoolPost, put: storagePoolPut, patch: storagePoolPatch, delete: storagePoolDelete}

// The storage pool structs and functions
type storagePool struct {
	// Properties
	daemon *Daemon
	id     int64
	poolName   string

	// config
	config map[string]string
}

func storagePoolLoadByName(d *Daemon, name string) (*storagePool, error) {
	id, dbInfo, err := dbStoragePoolGet(d.db, name)
	if err != nil {
		return nil, err
	}

	s := storagePool{daemon: d, id: id, poolName: name, config: dbInfo.Config}

	return &s, nil
}

func (s *storagePool) storagePoolCreate() error {
	size := ""
	if s.config["driver"] != "dir" {
		if s.config["size"] == "" {
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
			s.config["size"] = size
		}

		if err := dbStoragePoolUpdate(s.daemon.db, s.poolName, s.config); err != nil {
			return fmt.Errorf("Failed to update database")
		}
	}

	if s.config["driver"] == "zfs" {
		return s.zfsPoolCreate()
	}

	return nil
}

func (s *storagePool) zfsPoolCreate() error {
	vdev := s.config["source"]
	if vdev == "" {
		vdev = shared.VarPath(s.poolName)
	}

	out, err := exec.LookPath("zfs")
	if err != nil || len(out) == 0 {
		return fmt.Errorf("The 'zfs' tool isn't available")
	}

	if !filepath.IsAbs(vdev) {
		// Probably a zpool or zfs dataset.
		if err := zfsPoolCheck(vdev); err != nil {
			return err
		}

		// Confirm that the pool is empty.
		subvols, err := zfsPoolListSubvolumes(vdev)
		if err != nil {
			return err
		}

		if len(subvols) > 0 {
			return fmt.Errorf("Provided ZFS pool (or dataset) isn't empty")
		}

		_ = loadModule("zfs")

		return nil
	} else {
		if !shared.IsBlockdevPath(vdev) {
			// This is likely a loop file.
			f, err := os.Create(vdev)
			if err != nil {
				return fmt.Errorf("Failed to open %s: %s", vdev, err)
			}

			err = f.Chmod(0600)
			if err != nil {
				return fmt.Errorf("Failed to chmod %s: %s", vdev, err)
			}

			size := int64(0)
			if s.config["size"] != "" {
				size, err = shared.ParseByteSizeString(s.config["size"])
				if err != nil {
					return err
				}
			}
			err = f.Truncate(size)
			if err != nil {
				return fmt.Errorf("Failed to create sparse file %s: %s", vdev, err)
			}

			err = f.Close()
			if err != nil {
				return fmt.Errorf("Failed to close %s: %s", vdev, err)
			}
		}
	}

	zpoolName := s.config["zfs.pool_name"]
	if zpoolName == "" {
		zpoolName = s.poolName
	}

	output, err := exec.Command(
		"zpool",
		"create", zpoolName, vdev,
		"-f", "-m", "none", "-O", "compression=on").CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to create the ZFS pool: %s", output)
	}

	return nil
}

func (s *storagePool) zfsPoolDelete() error {
	zpoolName := s.config["zfs.pool_name"]
	if zpoolName == "" {
		zpoolName = s.poolName
	}

	// TODO: Check if any containers or images are using the pool.
	output, err := exec.Command(
		"zpool",
		"destroy", zpoolName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to delete the ZFS pool: %s", output)
	}

	// Cleanup storage
	vdev := s.config["source"]
	if filepath.IsAbs(vdev) && !shared.IsBlockdevPath(vdev) {
		os.RemoveAll(vdev)
	}

	return nil
}

// zfs functions.
func zfsPoolCheck(pool string) error {
	output, err := exec.Command(
		"zfs", "get", "type", "-H", "-o", "value", pool).CombinedOutput()
	if err != nil {
		return fmt.Errorf(strings.Split(string(output), "\n")[0])
	}

	poolType := strings.Split(string(output), "\n")[0]
	if poolType != "filesystem" {
		return fmt.Errorf("Unsupported pool type: %s", poolType)
	}

	return nil
}

func zfsPoolListSubvolumes(path string) ([]string, error) {
	output, err := exec.Command(
		"zfs",
		"list",
		"-t", "filesystem",
		"-o", "name",
		"-H",
		"-r", path).CombinedOutput()
	if err != nil {
		// s.log.Error("zfs list failed", log.Ctx{"output": string(output)})
		return []string{}, fmt.Errorf("Failed to list ZFS filesystems: %s", output)
	}

	children := []string{}
	for _, entry := range strings.Split(string(output), "\n") {
		if entry == "" {
			continue
		}

		if entry == path {
			continue
		}

		children = append(children, strings.TrimPrefix(entry, fmt.Sprintf("%s/", path)))
	}

	return children, nil
}

func (s *storagePool) zfsPoolRename(oldname string, newname string, poolOnly bool) error {

	// TODO: Check if any containers or images are using the pool.
	output, err := exec.Command(
		"zpool",
		"export", oldname).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to delete the ZFS pool: %s", output)
	}

	output, err = exec.Command(
		"zpool",
		"import", oldname, newname).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to delete the ZFS pool: %s", output)
	}

	// Cleanup storage
	vdev := s.config["source"]
	if filepath.IsAbs(vdev) && !shared.IsBlockdevPath(vdev) && !poolOnly {
		nvdev := newname
		if !filepath.IsAbs(newname) {
			nvdev = filepath.Join(filepath.Dir(vdev), newname)
		}
		os.Rename(vdev, nvdev)
	}

	return nil
}

func (s *storagePool) storagePoolRename(name string) error {
	// Rename the database entry
	err := dbStoragePoolRename(s.daemon.db, s.poolName, name)
	if err != nil {
		return err
	}

	if s.config["driver"] == "zfs" {
		return s.zfsPoolRename(s.poolName, name, s.config["zfs.pool_name"] == "")
	}

	return nil
}

func doStoragePoolUpdate(d *Daemon, name string, oldConfig map[string]string, newConfig map[string]string) Response {
	// Validate the configuration
	err := storagePoolValidateConfig(name, newConfig)
	if err != nil {
		return BadRequest(err)
	}

	// Load the existing storage pool.
	s, err := storagePoolLoadByName(d, name)
	if err != nil {
		return NotFound
	}

	if s.config["driver"] == "zfs" {
		err := s.zfsPoolUpdate(shared.StoragePoolConfig{Config: newConfig})
		if err != nil {
			return SmartError(err)
		}
	}

	return EmptySyncResponse
}

func (s *storagePool) zfsPoolUpdate(newPool shared.StoragePoolConfig) error {
	newConfig := newPool.Config

	// Backup the current state
	oldConfig := map[string]string{}
	err := shared.DeepCopy(&s.config, &oldConfig)
	if err != nil {
		return err
	}

	// Define a function which reverts everything.  Defer this function
	// so that it doesn't need to be explicitly called in every failing
	// return path.  Track whether or not we want to undo the changes
	// using a closure.
	undoChanges := true
	defer func() {
		if undoChanges {
			s.config = oldConfig
		}
	}()

	// Diff the configurations
	changedConfig := []string{}
	userOnly := true
	for key, _ := range oldConfig {
		if oldConfig[key] != newConfig[key] {
			if !strings.HasPrefix(key, "user.") {
				userOnly = false
			}

			if !shared.StringInSlice(key, changedConfig) {
				changedConfig = append(changedConfig, key)
			}
		}
	}

	for key, _ := range newConfig {
		if oldConfig[key] != newConfig[key] {
			if !strings.HasPrefix(key, "user.") {
				userOnly = false
			}

			if !shared.StringInSlice(key, changedConfig) {
				changedConfig = append(changedConfig, key)
			}
		}
	}

	// Skip on no change
	if len(changedConfig) == 0 {
		return nil
	}

	// Update the storage pool
	if !userOnly {
		if shared.StringInSlice("driver", changedConfig) {
			return fmt.Errorf("You cannot change the driver of a storage pool")
		}

		if shared.StringInSlice("size", changedConfig) {
			return fmt.Errorf("You cannot change the size of a storage pool")
		}

		if shared.StringInSlice("source", changedConfig) {
			return fmt.Errorf("You cannot change the source of a storage pool")
		}
	}

	// Apply the new configuration
	s.config = newConfig

	// Update the database
	err = dbStoragePoolUpdate(s.daemon.db, s.poolName, s.config)
	if err != nil {
		return err
	}

	if s.config["zfs.pool_name"] != "" && s.config["zfs.pool_name"] != oldConfig["zfs.pool_name"] {
		err := s.zfsPoolRename(oldConfig["zfs.pool_name"], s.config["zfs.pool_name"], true)
		if err != nil {
			return err
		}
	}

	// Success, update the closure to mark that the changes should be kept.
	undoChanges = false

	return nil
}
