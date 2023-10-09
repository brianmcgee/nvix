package directory

import (
	"bytes"
	"strings"

	capb "code.tvl.fyi/tvix/castore/protos"
	"github.com/juju/errors"
)

const (
	selfReference   = "."
	parentReference = ".."

	ErrEmptyName               = errors.ConstError("name cannot be an empty string")
	ErrNameWithSlash           = errors.ConstError("name cannot contain slashes: '/'")
	ErrNameWithNullByte        = errors.ConstError("name cannot contain null bytes")
	ErrNameWithSelfReference   = errors.ConstError("name cannot be a self reference: '.'")
	ErrNameWithParentReference = errors.ConstError("name cannot be a parent reference: '..'")
	ErrNamesAreNotSorted       = errors.ConstError("names must be lexicographically sorted")
)

type DirEntry interface {
	GetName() []byte
}

func validateDirectory(directory *capb.Directory) error {
	cache := make(map[string]DirEntry)

	validate := func(name string, entry DirEntry) error {
		if err := validateName(name); err != nil {
			return err
		} else if _, ok := cache[name]; ok {
			return errors.Errorf("duplicate name: %v", name)
		}
		cache[name] = entry
		return nil
	}

	var lastName []byte
	for _, dir := range directory.Directories {
		if err := validate(string(dir.Name), dir); err != nil {
			return err
		} else if len(lastName) > 0 && bytes.Compare(lastName, dir.Name) > 0 {
			return ErrNamesAreNotSorted
		}
		lastName = dir.Name
	}

	lastName = nil
	for _, file := range directory.Files {
		if err := validate(string(file.Name), file); err != nil {
			return err
		} else if len(lastName) > 0 && bytes.Compare(lastName, file.Name) > 0 {
			return ErrNamesAreNotSorted
		}
		lastName = file.Name
	}

	lastName = nil
	for _, link := range directory.Symlinks {
		if err := validate(string(link.Name), link); err != nil {
			return err
		} else if len(lastName) > 0 && bytes.Compare(lastName, link.Name) > 0 {
			return ErrNamesAreNotSorted
		}
		lastName = link.Name
	}

	return nil
}

func validateName(name string) error {
	if name == "" {
		return ErrEmptyName
	} else if name == selfReference {
		return ErrNameWithSelfReference
	} else if name == parentReference {
		return ErrNameWithParentReference
	} else if strings.Contains(name, "/") {
		return ErrNameWithSlash
	} else if strings.IndexByte(name, '\x00') > -1 {
		return ErrNameWithNullByte
	}

	return nil
}
