//go:build windows || linux

package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func packagedInstallationProcessProbe(fileSystem factorydefinitions.PackagedInstallationFileSystem) factorydefinitions.PackagedInstallationProcessProbe {
	return (platformprocess.IncarnationProbe{ReadFile: fileSystem.ReadFile}).LookupProcess
}
