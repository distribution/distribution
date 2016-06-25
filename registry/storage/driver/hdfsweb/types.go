// Refer to github.com/vladimirvivien/gowfs

package hdfsweb

import (
	"errors"
	"fmt"
)

const (
	// ExceptionIllegalArgument indicates 400 Bad Request
	ExceptionIllegalArgument = "IllegalArgumentException"

	// ExceptionUnsupportedOperation indicates 400 Bad Request
	ExceptionUnsupportedOperation = "UnsupportedOperationException"

	// ExceptionSecurity indicates 401 Unauthorized
	ExceptionSecurity = "SecurityException"

	// ExceptionIO indicates 403 Forbidden
	ExceptionIO = "IOException"

	// ExceptionFileNotFound indicates 404 Not Found
	ExceptionFileNotFound = "FileNotFoundException"

	// ExceptionRuntime indicates 500 Internal Server Error
	ExceptionRuntime = "RuntimeException"
)

// ErrBoolean means returning an error when the operation get
// a false result from Boolean JSON Schema.
var ErrBoolean = errors.New("Return a false boolean result")

// FileStatus Represents HDFS FileStatus (FileSystem.getStatus())
// See http://hadoop.apache.org/docs/r2.2.0/hadoop-project-dist/hadoop-hdfs/WebHDFS.html#FileStatus_JSON_Schema
// Example:
// {
//   "FileStatus":
//   {
//     "accessTime"      : 0, 				// integer
//     "blockSize"       : 0, 				// integer
//     "group"           : "grp",			// string
//     "length"          : 0,             	// integer - zero for directories
//     "modificationTime": 1320173277227,	// integer
//     "owner"           : "webuser",		// string
//     "pathSuffix"      : "",				// string
//     "permission"      : "777",			// string
//     "replication"     : 0,				// integer
//     "type"            : "DIRECTORY"    	// string - enum {FILE, DIRECTORY, SYMLINK}
//   }
// }
type FileStatus struct {
	AccesTime        int64  `json:"accessTime,omitempty"`
	BlockSize        int64  `json:"blockSize,omitempty"`
	Group            string `json:"group,omitempty"`
	Length           int64  `json:"length,omitempty"`
	ModificationTime int64  `json:"modificationTime,omitempty"`
	Owner            string `json:"owner,omitempty"`
	PathSuffix       string `json:"pathSuffix,omitempty"`
	Permission       string `json:"permission,omitempty"`
	Replication      int64  `json:"replication,omitempty"`
	Type             string `json:"type,omitempty"`
}

// FileStatuses Container type for multiple FileStatus for directory, etc
// (see HDFS FileSystem.listStatus())
type FileStatuses struct {
	FileStatus []FileStatus `json:"FileStatus,omitempty"`
}

// RemoteException defines the JSON schema of error responses.
// When the HTTP request to HDFS server get not OK or Created status, API
// functions return printed error message with the status code. It doesn't paser and
// return the RemoteException error content. The detailed exception content refers to:
// http://hadoop.apache.org/docs/current/hadoop-project-dist/hadoop-hdfs/WebHDFS.html#Error_Responses
// Example:
// Content-Type: application/json
// Transfer-Encoding: chunked
// {
//   "RemoteException":
//   {
//     "exception"    : "FileNotFoundException",
//     "javaClassName": "java.io.FileNotFoundException",
//     "message"      : "File does not exist: /foo/a.patch"
//   }
// }
type RemoteException struct {
	Exception     string `json:"exception,omitempty"`
	JavaClassName string `json:"javaClassName,omitempty"`
	Message       string `json:"message,omitempty"`
}

// Implementation of error type. it returns string representation of RemoteException.
func (re RemoteException) Error() string {
	return fmt.Sprintf("RemoteException = %s, JavaClassName = %s, Message = %s",
		re.Exception,
		re.JavaClassName,
		re.Message)
}

// RespException wrap the structure RemoteException
type RespException struct {
	RemoteException RemoteException `json:"RemoteException,omitempty"`
}
