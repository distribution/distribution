package bunnystorage

type Object struct {
	GUID            string `json:"Guid,omitempty"`
	StorageZoneName string `json:"StorageZoneName,omitempty"`
	Path            string `json:"Path,omitempty"`
	ObjectName      string `json:"ObjectName,omitempty"`
	Length          int    `json:"Length,omitempty"`
	LastChanged     string `json:"LastChanged,omitempty"`
	ServerID        int    `json:"ServerId,omitempty"`
	ArrayNumber     int    `json:"ArrayNumber,omitempty"`
	IsDirectory     bool   `json:"IsDirectory,omitempty"`
	UserID          string `json:"UserId,omitempty"`
	ContentType     string `json:"ContentType,omitempty"`
	DateCreated     string `json:"DateCreated,omitempty"`
	StorageZoneID   int    `json:"StorageZoneId,omitempty"`
	Checksum        string `json:"Checksum,omitempty"`
	ReplicatedZones string `json:"ReplicatedZones,omitempty"`
}
