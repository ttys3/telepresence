package logging

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hectane/go-acl/api"
	"golang.org/x/sys/windows"
)

func dupToStd(_ *os.File) interface{} {
	return errors.New("dupToStd() is not implemented on windows")
}

type windowsSysInfo struct {
	path    string
	data    *syscall.Win32FileAttributeData
	owner   *windows.SID
	group   *windows.SID
	dacl    windows.Handle
	sacl    windows.Handle
	secDesc windows.Handle
}

func (wi *windowsSysInfo) setOwnerAndGroup(name string) error {
	return api.SetNamedSecurityInfo(wi.path, api.SE_FILE_OBJECT, api.OWNER_SECURITY_INFORMATION, wi.owner, wi.group, wi.dacl, wi.sacl)
}

func (wi *windowsSysInfo) haveSameOwnerAndGroup(s sysinfo) bool {
	owi, ok := s.(*windowsSysInfo)
	return ok && wi.owner.Equals(owi.owner) && wi.group.Equals(owi.group)
}

func getSysInfo(dir string, info os.FileInfo) sysinfo {
	wi := windowsSysInfo{
		path: filepath.Join(dir, info.Name()),
		data: info.Sys().(*syscall.Win32FileAttributeData),
	}
	err := api.GetNamedSecurityInfo(
		wi.path,
		api.SE_FILE_OBJECT,
		api.OWNER_SECURITY_INFORMATION,
		&wi.owner,
		&wi.group,
		&wi.dacl,
		&wi.sacl,
		&wi.secDesc,
	)
	if err != nil {
		panic(err)
	}
	return &wi
}

func (wi *windowsSysInfo) birthtime() time.Time {
	return time.Unix(0, wi.data.CreationTime.Nanoseconds())
}
