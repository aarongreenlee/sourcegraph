package fs

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"src.sourcegraph.com/sourcegraph/vendored/encoding/base64" // TODO: Replace with "encoding/base64" in Go 1.5.

	"golang.org/x/net/context"
	"sourcegraph.com/sourcegraph/go-sourcegraph/sourcegraph"
	"src.sourcegraph.com/sourcegraph/store"
)

// userKeys is an FS-backed implementation of the UserKeys store.
type userKeys struct {
	// dir is the system filepath to the root directory of the keys store.
	dir string
}

func NewUserKeys() store.UserKeys {
	dir := filepath.Join(os.Getenv("SGPATH"), "db", "user_keys", "keys")
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		log.Fatalf("creating directory %q failed: %v", dir, err)
	}

	return &userKeys{
		dir: dir,
	}
}

func (s *userKeys) AddKey(_ context.Context, uid int32, key sourcegraph.SSHPublicKey) error {
	dir := s.hashDirForKey(key)

	_, err := os.Stat(filepath.Join(dir, fmt.Sprint(uid)))
	if !os.IsNotExist(err) {
		return os.ErrExist
	}

	err = os.Mkdir(dir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("creating directory %q failed: %v", dir, err)
	}

	err = ioutil.WriteFile(filepath.Join(dir, fmt.Sprint(uid)), key.Key, 0644)
	if err != nil {
		return err
	}

	return nil
}

func (s *userKeys) LookupUser(_ context.Context, key sourcegraph.SSHPublicKey) (*sourcegraph.UserSpec, error) {
	dir := s.hashDirForKey(key)

	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, fi := range fis {
		b, err := ioutil.ReadFile(filepath.Join(dir, fi.Name()))
		if err != nil {
			return nil, err
		}

		if bytes.Equal(b, key.Key) {
			uid, err := strconv.ParseInt(fi.Name(), 10, 32)
			if err != nil {
				return nil, err
			}
			return &sourcegraph.UserSpec{UID: int32(uid)}, nil
		}
	}

	return nil, fmt.Errorf("user with given key not found")
}

func (s *userKeys) DeleteKey(_ context.Context, uid int32) error {
	dirs, err := ioutil.ReadDir(s.dir)
	if err != nil {
		return err
	}

	for _, dir := range dirs {
		hashDir := filepath.Join(s.dir, dir.Name())

		err := os.Remove(filepath.Join(hashDir, fmt.Sprint(uid)))
		if err == nil {
			// Rmdir if now empty.
			if fis, err := ioutil.ReadDir(hashDir); err == nil && len(fis) == 0 {
				_ = os.Remove(hashDir)
			}

			// Successfully return.
			return nil
		}
	}

	return os.ErrNotExist
}

func (s *userKeys) hashDirForKey(key sourcegraph.SSHPublicKey) string {
	return filepath.Join(s.dir, publicKeyToHash(key.Key))
}

func publicKeyToHash(key []byte) string {
	h := sha1.New()
	_, err := h.Write(key)
	if err != nil {
		panic(err) // This is expected to never happen.
	}
	sum := h.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(sum)
}
