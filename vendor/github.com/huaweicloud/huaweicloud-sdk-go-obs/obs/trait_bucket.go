// Copyright 2019 Huawei Technologies Co.,Ltd.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use
// this file except in compliance with the License.  You may obtain a copy of the
// License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations under the License.

package obs

import (
	"fmt"
	"strings"
)

func (input ListBucketsInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	headers = make(map[string][]string)
	if input.QueryLocation && !isObs {
		setHeaders(headers, HEADER_LOCATION_AMZ, []string{"true"}, isObs)
	}
	if input.BucketType != "" {
		setHeaders(headers, HEADER_BUCKET_TYPE, []string{string(input.BucketType)}, true)
	}
	return
}

func (input CreateBucketInput) prepareGrantHeaders(headers map[string][]string, isObs bool) {
	if grantReadID := input.GrantReadId; grantReadID != "" {
		setHeaders(headers, HEADER_GRANT_READ_OBS, []string{grantReadID}, isObs)
	}
	if grantWriteID := input.GrantWriteId; grantWriteID != "" {
		setHeaders(headers, HEADER_GRANT_WRITE_OBS, []string{grantWriteID}, isObs)
	}
	if grantReadAcpID := input.GrantReadAcpId; grantReadAcpID != "" {
		setHeaders(headers, HEADER_GRANT_READ_ACP_OBS, []string{grantReadAcpID}, isObs)
	}
	if grantWriteAcpID := input.GrantWriteAcpId; grantWriteAcpID != "" {
		setHeaders(headers, HEADER_GRANT_WRITE_ACP_OBS, []string{grantWriteAcpID}, isObs)
	}
	if grantFullControlID := input.GrantFullControlId; grantFullControlID != "" {
		setHeaders(headers, HEADER_GRANT_FULL_CONTROL_OBS, []string{grantFullControlID}, isObs)
	}
	if grantReadDeliveredID := input.GrantReadDeliveredId; grantReadDeliveredID != "" {
		setHeaders(headers, HEADER_GRANT_READ_DELIVERED_OBS, []string{grantReadDeliveredID}, true)
	}
	if grantFullControlDeliveredID := input.GrantFullControlDeliveredId; grantFullControlDeliveredID != "" {
		setHeaders(headers, HEADER_GRANT_FULL_CONTROL_DELIVERED_OBS, []string{grantFullControlDeliveredID}, true)
	}
}

func (input CreateBucketInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	headers = make(map[string][]string)
	if acl := string(input.ACL); acl != "" {
		setHeaders(headers, HEADER_ACL, []string{acl}, isObs)
	}
	if storageClass := string(input.StorageClass); storageClass != "" {
		if !isObs {
			if storageClass == string(StorageClassWarm) {
				storageClass = string(storageClassStandardIA)
			} else if storageClass == string(StorageClassCold) {
				storageClass = string(storageClassGlacier)
			}
		}
		setHeadersNext(headers, HEADER_STORAGE_CLASS_OBS, HEADER_STORAGE_CLASS, []string{storageClass}, isObs)
	}
	if epid := input.Epid; epid != "" {
		setHeaders(headers, HEADER_EPID_HEADERS, []string{epid}, isObs)
	}
	if availableZone := input.AvailableZone; availableZone != "" {
		setHeaders(headers, HEADER_AZ_REDUNDANCY, []string{availableZone}, isObs)
	}

	input.prepareGrantHeaders(headers, isObs)
	if input.IsFSFileInterface {
		setHeaders(headers, headerFSFileInterface, []string{"Enabled"}, true)
	}

	if location := strings.TrimSpace(input.Location); location != "" {
		input.Location = location

		xml := make([]string, 0, 3)
		xml = append(xml, "<CreateBucketConfiguration>")
		if isObs {
			xml = append(xml, fmt.Sprintf("<Location>%s</Location>", input.Location))
		} else {
			xml = append(xml, fmt.Sprintf("<LocationConstraint>%s</LocationConstraint>", input.Location))
		}
		xml = append(xml, "</CreateBucketConfiguration>")

		data = strings.Join(xml, "")
	}

	if bucketRedundancy := string(input.BucketRedundancy); bucketRedundancy != "" {
		setHeaders(headers, HEADER_BUCKET_REDUNDANCY, []string{bucketRedundancy}, isObs)
	}
	if input.IsFusionAllowUpgrade {
		setHeaders(headers, HEADER_FUSION_ALLOW_UPGRADE, []string{"true"}, isObs)
	}

	if input.IsRedundancyAllowALT {
		setHeaders(headers, HEADER_FUSION_ALLOW_ALT, []string{"true"}, isObs)
	}

	return
}

func (input SetBucketStoragePolicyInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	xml := make([]string, 0, 1)
	if !isObs {
		storageClass := "STANDARD"
		if input.StorageClass == StorageClassWarm {
			storageClass = string(storageClassStandardIA)
		} else if input.StorageClass == StorageClassCold {
			storageClass = string(storageClassGlacier)
		}
		params = map[string]string{string(SubResourceStoragePolicy): ""}
		xml = append(xml, fmt.Sprintf("<StoragePolicy><DefaultStorageClass>%s</DefaultStorageClass></StoragePolicy>", storageClass))
	} else {
		if input.StorageClass != StorageClassWarm && input.StorageClass != StorageClassCold {
			input.StorageClass = StorageClassStandard
		}
		params = map[string]string{string(SubResourceStorageClass): ""}
		xml = append(xml, fmt.Sprintf("<StorageClass>%s</StorageClass>", input.StorageClass))
	}
	data = strings.Join(xml, "")
	return
}

func (input SetBucketQuotaInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	return trans(SubResourceQuota, input)
}

func (input SetBucketAclInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourceAcl): ""}
	headers = make(map[string][]string)

	if acl := string(input.ACL); acl != "" {
		setHeaders(headers, HEADER_ACL, []string{acl}, isObs)
	} else {
		data, _ = convertBucketACLToXML(input.AccessControlPolicy, false, isObs)
	}
	return
}

func (input SetBucketPolicyInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourcePolicy): ""}
	data = strings.NewReader(input.Policy)
	return
}

func (input SetBucketCorsInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourceCors): ""}
	data, md5, err := ConvertRequestToIoReaderV2(input)
	if err != nil {
		return
	}
	headers = map[string][]string{HEADER_MD5_CAMEL: {md5}}
	return
}

func (input SetBucketVersioningInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	return trans(SubResourceVersioning, input)
}

func (input SetBucketWebsiteConfigurationInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourceWebsite): ""}
	data, _ = ConvertWebsiteConfigurationToXml(input.BucketWebsiteConfiguration, false)
	return
}

func (input GetBucketMetadataInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	headers = make(map[string][]string)
	if origin := strings.TrimSpace(input.Origin); origin != "" {
		headers[HEADER_ORIGIN_CAMEL] = []string{origin}
	}
	if requestHeader := strings.TrimSpace(input.RequestHeader); requestHeader != "" {
		headers[HEADER_ACCESS_CONTROL_REQUEST_HEADER_CAMEL] = []string{requestHeader}
	}
	return
}

func (input SetBucketLoggingConfigurationInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourceLogging): ""}
	data, _ = ConvertLoggingStatusToXml(input.BucketLoggingStatus, false, isObs)
	return
}

func (input SetBucketLifecycleConfigurationInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourceLifecycle): ""}
	data, md5 := ConvertLifecyleConfigurationToXml(input.BucketLifecyleConfiguration, true, isObs)
	headers = map[string][]string{HEADER_MD5_CAMEL: {md5}}
	return
}

func (input SetBucketEncryptionInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourceEncryption): ""}
	data, _ = ConvertEncryptionConfigurationToXml(input.BucketEncryptionConfiguration, false, isObs)
	return
}

func (input SetBucketTaggingInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourceTagging): ""}
	data, md5, err := ConvertRequestToIoReaderV2(input)
	if err != nil {
		return
	}
	headers = map[string][]string{HEADER_MD5_CAMEL: {md5}}
	return
}

func (input SetBucketNotificationInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourceNotification): ""}
	data, _ = ConvertNotificationToXml(input.BucketNotification, false, isObs)
	return
}

func (input SetBucketRequestPaymentInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	return trans(SubResourceRequestPayment, input)
}

func (input SetBucketFetchPolicyInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	contentType, _ := mimeTypes["json"]
	headers = make(map[string][]string, 2)
	headers[HEADER_CONTENT_TYPE] = []string{contentType}
	setHeaders(headers, headerOefMarker, []string{"yes"}, isObs)
	data, err = convertFetchPolicyToJSON(input)
	return
}

func (input GetBucketFetchPolicyInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	headers = make(map[string][]string, 1)
	setHeaders(headers, headerOefMarker, []string{"yes"}, isObs)
	return
}

func (input DeleteBucketFetchPolicyInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	headers = make(map[string][]string, 1)
	setHeaders(headers, headerOefMarker, []string{"yes"}, isObs)
	return
}

func (input SetBucketFetchJobInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	contentType, _ := mimeTypes["json"]
	headers = make(map[string][]string, 2)
	headers[HEADER_CONTENT_TYPE] = []string{contentType}
	setHeaders(headers, headerOefMarker, []string{"yes"}, isObs)
	data, err = convertFetchJobToJSON(input)
	return
}

func (input GetBucketFetchJobInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	headers = make(map[string][]string, 1)
	setHeaders(headers, headerOefMarker, []string{"yes"}, isObs)
	return
}

func (input SetBucketMirrorBackToSourceInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	params = map[string]string{string(SubResourceMirrorBackToSource): ""}

	contentType, _ := mimeTypes["json"]
	headers = make(map[string][]string, 1)
	headers[HEADER_CONTENT_TYPE] = []string{contentType}
	data = input.Rules
	return
}

func (input DeleteBucketCustomDomainInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	return trans(SubResourceCustomDomain, input)
}

func (input SetBucketCustomDomainInput) trans(isObs bool) (params map[string]string, headers map[string][]string, data interface{}, err error) {
	return trans(SubResourceCustomDomain, input)
}
