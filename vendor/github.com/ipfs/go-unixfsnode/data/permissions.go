package data

const FilePermissionsDefault = 0o0644
const DirectorPerimissionsDefault = 0o0755
const HAMTShardPerimissionsDefault = 0o0755

func (u UnixFSData) Permissions() int {
	if u.FieldMode().Exists() {
		return int(u.FieldMode().Must().Int() & 0xFFF)
	}
	return DefaultPermissions(u)
}

// DefaultPermissions gets the default permissions for a UnixFS object based on its
// type
func DefaultPermissions(u UnixFSData) int {
	switch u.FieldDataType().Int() {
	case Data_File:
		return FilePermissionsDefault
	case Data_Directory:
		return DirectorPerimissionsDefault
	case Data_HAMTShard:
		return HAMTShardPerimissionsDefault
	default:
		return 0
	}
}
