// +build !windows

package logging

import (
	"os"

	"golang.org/x/sys/unix"
)

type unixSysInfo unix.Stat_t

func getSysInfo(_ string, info os.FileInfo) sysinfo {
	return (*unixSysInfo)(info.Sys().(*unix.Stat_t))
}

func (u *unixSysInfo) setOwnerAndGroup(name string) error {
	return os.Chown(name, int(u.Uid), int(u.Gid))
}

func (u *unixSysInfo) haveSameOwnerAndGroup(other sysinfo) bool {
	ou := other.(*unixSysInfo)
	return u.Uid == ou.Uid && u.Gid == ou.Gid
}
