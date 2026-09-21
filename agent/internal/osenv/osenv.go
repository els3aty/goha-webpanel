package osenv

import (
	"os"
	"strings"
	"sync"
)

type Family string

const (
	FamilyDebian Family = "debian"
	FamilyRHEL   Family = "rhel"
	FamilyUnknown Family = "unknown"
)

type Environment interface {
	Family() Family
	PackageManager() string
	NginxVhostDir() string
	NginxSymlinkDir() string // Empty if symlinks aren't used (like RHEL)
	NginxService() string
	PHPFPMPoolDir(version string) string
	PHPFPMService(version string) string
}

type defaultEnv struct {
	family Family
}

var (
	currentEnv Environment
	once       sync.Once
)

// Get returns the detected OS environment configuration
func Get() Environment {
	once.Do(func() {
		currentEnv = detectOS()
	})
	return currentEnv
}

func detectOS() Environment {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		// Fallback to Debian defaults if we can't read os-release
		return &defaultEnv{family: FamilyDebian}
	}

	content := strings.ToLower(string(data))
	if strings.Contains(content, "id=ubuntu") || strings.Contains(content, "id=debian") {
		return &defaultEnv{family: FamilyDebian}
	} else if strings.Contains(content, "id=\"almalinux\"") || strings.Contains(content, "id=\"rocky\"") || strings.Contains(content, "id=\"centos\"") || strings.Contains(content, "id=\"rhel\"") || strings.Contains(content, "id=almalinux") {
		return &defaultEnv{family: FamilyRHEL}
	}

	return &defaultEnv{family: FamilyDebian}
}

func (e *defaultEnv) Family() Family {
	return e.family
}

func (e *defaultEnv) PackageManager() string {
	if e.family == FamilyRHEL {
		return "dnf"
	}
	return "apt-get"
}

func (e *defaultEnv) NginxVhostDir() string {
	if e.family == FamilyRHEL {
		return "/etc/nginx/conf.d"
	}
	return "/etc/nginx/sites-available"
}

func (e *defaultEnv) NginxSymlinkDir() string {
	if e.family == FamilyRHEL {
		return "" // RHEL doesn't use sites-enabled by default
	}
	return "/etc/nginx/sites-enabled"
}

func (e *defaultEnv) NginxService() string {
	return "nginx"
}

func (e *defaultEnv) PHPFPMPoolDir(version string) string {
	if e.family == FamilyRHEL {
		// Remi repo usually puts fpm pools in /etc/opt/remi/php82/php-fpm.d/ or /etc/php-fpm.d/
		// We standardize on /etc/opt/remi/phpXX/php-fpm.d/
		verCompact := strings.ReplaceAll(version, ".", "")
		return "/etc/opt/remi/php" + verCompact + "/php-fpm.d"
	}
	return "/etc/php/" + version + "/fpm/pool.d"
}

func (e *defaultEnv) PHPFPMService(version string) string {
	if e.family == FamilyRHEL {
		verCompact := strings.ReplaceAll(version, ".", "")
		return "php" + verCompact + "-php-fpm"
	}
	return "php" + version + "-fpm"
}
