package swift

import (
	"fmt"
	"strings"
	"time"

	"github.com/ncw/swift"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

//An object that is/was uploaded in segments, which are then combined through a
//manifest. Swift clusters support DLO (Dynamic Large Object) and sometimes SLO
//(Static Large Object). These methods are abstracted by the `handler`
//interface.
type largeObject struct {
	driver       *driver
	swiftPath    string
	segmentsPath string
	segments     []swift.Object
	handler      largeObjectHandler
}

//Creates a new empty large object. A handler is chosen according to the
//cluster's capabilities.
func (d *driver) makeLargeObject(swiftPath, segmentsPath string) *largeObject {
	return &largeObject{d, swiftPath, segmentsPath, nil, &dloHandler{}}
}

//Take the return values of a `swift.Connection.Object()` call and parse the
//manifest of this large object. If the given object is not a large object, a
//pair of `nil` is returned.
func (d *driver) parseLargeObject(info swift.Object, headers swift.Headers) (*largeObject, error) {
	handlers := []largeObjectHandler{&dloHandler{}}

	//see if any handler can parse this large object
	for _, handler := range handlers {
		obj := &largeObject{d, info.Name, "", nil, nil}
		success, err := handler.parseLargeObject(obj, info, headers)
		if success || err != nil {
			obj.handler = handler
			return obj, err
		}
	}

	//nope, not a large object that we can handle
	return nil, nil
}

//The combined size of all segments in bytes.
func (obj *largeObject) Size() int64 {
	var size int64
	for _, segment := range obj.segments {
		size += segment.Bytes
	}
	return size
}

//Where to put the next segment of this large object.
func (obj *largeObject) NextSegmentPath() string {
	segmentNumber := len(obj.segments) + 1
	return fmt.Sprintf("%s/%016d", obj.segmentsPath, segmentNumber)
}

//Create a new manifest for this object which references all the segments in
//this instance.
func (obj *largeObject) WriteManifest() error {
	return obj.handler.writeManifest(obj)
}

//A largeObjectHandler describes how to parse and write a particular type of
//manifests.
type largeObjectHandler interface {
	//ParseLargeObject works like the global-scope method of the same name, but
	//limited to this particular handler. It fills the segmentsPath and segments
	//attributes on a partially initialized LargeObject.
	parseLargeObject(obj *largeObject, info swift.Object, headers swift.Headers) (handled bool, e error)
	//WriteManifest writes the manifest for this object, including all segments
	//that were added to it.
	writeManifest(obj *largeObject) error
}

type dloHandler struct{}

func (h *dloHandler) parseLargeObject(obj *largeObject, info swift.Object, headers swift.Headers) (bool, error) {
	manifest, ok := headers["X-Object-Manifest"]
	if !ok {
		//not a DLO
		return false, nil
	}

	//manifest == "$container/$segmentsPath"; extract segments path
	components := strings.SplitN(manifest, "/", 2)
	if len(components) > 1 {
		obj.segmentsPath = components[1]
	}

	//list segments - a simple container listing works 99.9% of the time
	d := obj.driver
	var err error
	obj.segments, err = d.Conn.ObjectsAll(d.Container, &swift.ObjectsOpts{Prefix: obj.segmentsPath})
	if err != nil {
		if err == swift.ContainerNotFound {
			return true, storagedriver.PathNotFoundError{Path: obj.segmentsPath}
		}
		return true, err
	}

	//build a lookup table by object name
	hasObjectName := make(map[string]struct{})
	for _, segment := range obj.segments {
		hasObjectName[segment.Name] = struct{}{}
	}

	//The container listing might be outdated (i.e. not contain all existing
	//segment objects yet) because of temporary inconsistency (Swift is only
	//eventually consistent!). Check its completeness.
	segmentNumber := 0
	for {
		segmentNumber++
		segmentPath := fmt.Sprintf("%s/%016d", obj.segmentsPath, segmentNumber)

		if _, seen := hasObjectName[segmentPath]; seen {
			continue
		}

		//This segment is missing in the container listing. Use a more reliable
		//request to check its existence. (HEAD requests on segments are
		//guaranteed to return the correct metadata, except for the pathological
		//case of an outage of large parts of the Swift cluster or its network,
		//since every segment is only written once.)
		segment, _, err := d.Conn.Object(d.Container, segmentPath)
		switch err {
		case nil:
			//found new segment -> keep going, more might be missing
			obj.segments = append(obj.segments, segment)
			continue
		case swift.ObjectNotFound:
			//This segment is missing. Since we upload segments sequentially,
			//there won't be any more segments after it.
			return true, nil
		default:
			return true, err //unexpected error
		}
	}
}

func (h *dloHandler) writeManifest(obj *largeObject) error {
	d := obj.driver
	headers := make(swift.Headers)
	headers["X-Object-Manifest"] = d.Container + "/" + obj.segmentsPath

	manifest, err := d.Conn.ObjectCreate(d.Container, obj.swiftPath, false, "", contentType, headers)
	if err != nil {
		if err == swift.ObjectNotFound {
			return storagedriver.PathNotFoundError{Path: obj.swiftPath}
		}
		return err
	}

	if err := manifest.Close(); err != nil {
		if err == swift.ObjectNotFound {
			return storagedriver.PathNotFoundError{Path: obj.swiftPath}
		}
		return err
	}

	//wait for segments to show up in container listing (which is updated
	//asynchronously)
	waitingTime := readAfterWriteWait
	endTime := time.Now().Add(readAfterWriteTimeout)
	for {
		var info swift.Object
		if info, _, err = d.Conn.Object(d.Container, obj.swiftPath); err == nil {
			if info.Bytes == obj.Size() {
				break
			}
			err = fmt.Errorf("Timeout expired while waiting for segments of %s to show up", obj.swiftPath)
		}
		if time.Now().Add(waitingTime).After(endTime) {
			break
		}
		time.Sleep(waitingTime)
		waitingTime *= 2
	}

	return err
}
