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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func cleanHeaderPrefix(header http.Header) map[string][]string {
	responseHeaders := make(map[string][]string)
	for key, value := range header {
		if len(value) > 0 {
			key = strings.ToLower(key)
			if strings.HasPrefix(key, HEADER_PREFIX) || strings.HasPrefix(key, HEADER_PREFIX_OBS) {
				key = key[len(HEADER_PREFIX):]
			}
			responseHeaders[key] = value
		}
	}
	return responseHeaders
}

// ParseStringToEventType converts string value to EventType value and returns it
func ParseStringToEventType(value string) (ret EventType) {
	switch value {
	case "ObjectCreated:*", "s3:ObjectCreated:*":
		ret = ObjectCreatedAll
	case "ObjectCreated:Put", "s3:ObjectCreated:Put":
		ret = ObjectCreatedPut
	case "ObjectCreated:Post", "s3:ObjectCreated:Post":
		ret = ObjectCreatedPost
	case "ObjectCreated:Copy", "s3:ObjectCreated:Copy":
		ret = ObjectCreatedCopy
	case "ObjectCreated:CompleteMultipartUpload", "s3:ObjectCreated:CompleteMultipartUpload":
		ret = ObjectCreatedCompleteMultipartUpload
	case "ObjectRemoved:*", "s3:ObjectRemoved:*":
		ret = ObjectRemovedAll
	case "ObjectRemoved:Delete", "s3:ObjectRemoved:Delete":
		ret = ObjectRemovedDelete
	case "ObjectRemoved:DeleteMarkerCreated", "s3:ObjectRemoved:DeleteMarkerCreated":
		ret = ObjectRemovedDeleteMarkerCreated
	default:
		ret = ""
	}
	return
}

// ParseStringToStorageClassType converts string value to StorageClassType value and returns it
func ParseStringToStorageClassType(value string) (ret StorageClassType) {
	switch value {
	case "STANDARD":
		ret = StorageClassStandard
	case "STANDARD_IA", "WARM":
		ret = StorageClassWarm
	case "GLACIER", "COLD":
		ret = StorageClassCold
	default:
		ret = ""
	}
	return
}

func ParseStringToFSStatusType(value string) (ret FSStatusType) {
	switch value {
	case "Enabled":
		ret = FSStatusEnabled
	case "Disabled":
		ret = FSStatusDisabled
	default:
		ret = ""
	}
	return
}

func prepareGrantURI(grant Grant) string {
	if grant.Grantee.URI == GroupAllUsers || grant.Grantee.URI == GroupAuthenticatedUsers {
		return fmt.Sprintf("<URI>%s%s</URI>", "http://acs.amazonaws.com/groups/global/", grant.Grantee.URI)
	}
	if grant.Grantee.URI == GroupLogDelivery {
		return fmt.Sprintf("<URI>%s%s</URI>", "http://acs.amazonaws.com/groups/s3/", grant.Grantee.URI)
	}
	return fmt.Sprintf("<URI>%s</URI>", grant.Grantee.URI)
}

func convertGrantToXML(grant Grant, isObs bool, isBucket bool) string {
	xml := make([]string, 0, 4)

	if grant.Grantee.Type == GranteeUser {
		if isObs {
			xml = append(xml, "<Grant><Grantee>")
		} else {
			xml = append(xml, fmt.Sprintf("<Grant><Grantee xsi:type=\"%s\" xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\">", grant.Grantee.Type))
		}
		if grant.Grantee.ID != "" {
			granteeID := XmlTranscoding(grant.Grantee.ID)
			xml = append(xml, fmt.Sprintf("<ID>%s</ID>", granteeID))
		}
		if !isObs && grant.Grantee.DisplayName != "" {
			granteeDisplayName := XmlTranscoding(grant.Grantee.DisplayName)
			xml = append(xml, fmt.Sprintf("<DisplayName>%s</DisplayName>", granteeDisplayName))
		}
		xml = append(xml, "</Grantee>")
	} else {
		if !isObs {
			xml = append(xml, fmt.Sprintf("<Grant><Grantee xsi:type=\"%s\" xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\">", grant.Grantee.Type))
			xml = append(xml, prepareGrantURI(grant))
			xml = append(xml, "</Grantee>")
		} else if grant.Grantee.URI == GroupAllUsers {
			xml = append(xml, "<Grant><Grantee>")
			xml = append(xml, fmt.Sprintf("<Canned>Everyone</Canned>"))
			xml = append(xml, "</Grantee>")
		} else {
			return strings.Join(xml, "")
		}
	}

	xml = append(xml, fmt.Sprintf("<Permission>%s</Permission>", grant.Permission))
	if isObs && isBucket {
		xml = append(xml, fmt.Sprintf("<Delivered>%t</Delivered>", grant.Delivered))
	}
	xml = append(xml, fmt.Sprintf("</Grant>"))
	return strings.Join(xml, "")
}

func hasLoggingTarget(input BucketLoggingStatus) bool {
	if input.TargetBucket != "" || input.TargetPrefix != "" || len(input.TargetGrants) > 0 {
		return true
	}
	return false
}

// ConvertLoggingStatusToXml converts BucketLoggingStatus value to XML data and returns it
func ConvertLoggingStatusToXml(input BucketLoggingStatus, returnMd5 bool, isObs bool) (data string, md5 string) {
	grantsLength := len(input.TargetGrants)
	xml := make([]string, 0, 8+grantsLength)

	xml = append(xml, "<BucketLoggingStatus>")
	if isObs && input.Agency != "" {
		agency := XmlTranscoding(input.Agency)
		xml = append(xml, fmt.Sprintf("<Agency>%s</Agency>", agency))
	}
	if hasLoggingTarget(input) {
		xml = append(xml, "<LoggingEnabled>")
		if input.TargetBucket != "" {
			xml = append(xml, fmt.Sprintf("<TargetBucket>%s</TargetBucket>", input.TargetBucket))
		}
		if input.TargetPrefix != "" {
			targetPrefix := XmlTranscoding(input.TargetPrefix)
			xml = append(xml, fmt.Sprintf("<TargetPrefix>%s</TargetPrefix>", targetPrefix))
		}
		if grantsLength > 0 {
			xml = append(xml, "<TargetGrants>")
			for _, grant := range input.TargetGrants {
				xml = append(xml, convertGrantToXML(grant, isObs, false))
			}
			xml = append(xml, "</TargetGrants>")
		}

		xml = append(xml, "</LoggingEnabled>")
	}
	xml = append(xml, "</BucketLoggingStatus>")
	data = strings.Join(xml, "")
	if returnMd5 {
		md5 = Base64Md5([]byte(data))
	}
	return
}

// ConvertAclToXml converts AccessControlPolicy value to XML data and returns it
func ConvertAclToXml(input AccessControlPolicy, returnMd5 bool, isObs bool) (data string, md5 string) {
	xml := make([]string, 0, 4+len(input.Grants))
	ownerID := XmlTranscoding(input.Owner.ID)
	xml = append(xml, fmt.Sprintf("<AccessControlPolicy><Owner><ID>%s</ID>", ownerID))
	if !isObs && input.Owner.DisplayName != "" {
		ownerDisplayName := XmlTranscoding(input.Owner.DisplayName)
		xml = append(xml, fmt.Sprintf("<DisplayName>%s</DisplayName>", ownerDisplayName))
	}
	if isObs && input.Delivered != "" {
		objectDelivered := XmlTranscoding(input.Delivered)
		xml = append(xml, fmt.Sprintf("</Owner><Delivered>%s</Delivered><AccessControlList>", objectDelivered))
	} else {
		xml = append(xml, "</Owner><AccessControlList>")
	}
	for _, grant := range input.Grants {
		xml = append(xml, convertGrantToXML(grant, isObs, false))
	}
	xml = append(xml, "</AccessControlList></AccessControlPolicy>")
	data = strings.Join(xml, "")
	if returnMd5 {
		md5 = Base64Md5([]byte(data))
	}
	return
}

func convertBucketACLToXML(input AccessControlPolicy, returnMd5 bool, isObs bool) (data string, md5 string) {
	xml := make([]string, 0, 4+len(input.Grants))
	ownerID := XmlTranscoding(input.Owner.ID)
	xml = append(xml, fmt.Sprintf("<AccessControlPolicy><Owner><ID>%s</ID>", ownerID))
	if !isObs && input.Owner.DisplayName != "" {
		ownerDisplayName := XmlTranscoding(input.Owner.DisplayName)
		xml = append(xml, fmt.Sprintf("<DisplayName>%s</DisplayName>", ownerDisplayName))
	}

	xml = append(xml, "</Owner><AccessControlList>")

	for _, grant := range input.Grants {
		xml = append(xml, convertGrantToXML(grant, isObs, true))
	}
	xml = append(xml, "</AccessControlList></AccessControlPolicy>")
	data = strings.Join(xml, "")
	if returnMd5 {
		md5 = Base64Md5([]byte(data))
	}
	return
}

func convertConditionToXML(condition Condition) string {
	xml := make([]string, 0, 2)
	if condition.KeyPrefixEquals != "" {
		keyPrefixEquals := XmlTranscoding(condition.KeyPrefixEquals)
		xml = append(xml, fmt.Sprintf("<KeyPrefixEquals>%s</KeyPrefixEquals>", keyPrefixEquals))
	}
	if condition.HttpErrorCodeReturnedEquals != "" {
		xml = append(xml, fmt.Sprintf("<HttpErrorCodeReturnedEquals>%s</HttpErrorCodeReturnedEquals>", condition.HttpErrorCodeReturnedEquals))
	}
	if len(xml) > 0 {
		return fmt.Sprintf("<Condition>%s</Condition>", strings.Join(xml, ""))
	}
	return ""
}

func prepareRoutingRule(input BucketWebsiteConfiguration) string {
	xml := make([]string, 0, len(input.RoutingRules)*10)
	for _, routingRule := range input.RoutingRules {
		xml = append(xml, "<RoutingRule>")
		xml = append(xml, "<Redirect>")
		if routingRule.Redirect.Protocol != "" {
			xml = append(xml, fmt.Sprintf("<Protocol>%s</Protocol>", routingRule.Redirect.Protocol))
		}
		if routingRule.Redirect.HostName != "" {
			xml = append(xml, fmt.Sprintf("<HostName>%s</HostName>", routingRule.Redirect.HostName))
		}
		if routingRule.Redirect.ReplaceKeyPrefixWith != "" {
			replaceKeyPrefixWith := XmlTranscoding(routingRule.Redirect.ReplaceKeyPrefixWith)
			xml = append(xml, fmt.Sprintf("<ReplaceKeyPrefixWith>%s</ReplaceKeyPrefixWith>", replaceKeyPrefixWith))
		}

		if routingRule.Redirect.ReplaceKeyWith != "" {
			replaceKeyWith := XmlTranscoding(routingRule.Redirect.ReplaceKeyWith)
			xml = append(xml, fmt.Sprintf("<ReplaceKeyWith>%s</ReplaceKeyWith>", replaceKeyWith))
		}
		if routingRule.Redirect.HttpRedirectCode != "" {
			xml = append(xml, fmt.Sprintf("<HttpRedirectCode>%s</HttpRedirectCode>", routingRule.Redirect.HttpRedirectCode))
		}
		xml = append(xml, "</Redirect>")

		if ret := convertConditionToXML(routingRule.Condition); ret != "" {
			xml = append(xml, ret)
		}
		xml = append(xml, "</RoutingRule>")
	}
	return strings.Join(xml, "")
}

// ConvertWebsiteConfigurationToXml converts BucketWebsiteConfiguration value to XML data and returns it
func ConvertWebsiteConfigurationToXml(input BucketWebsiteConfiguration, returnMd5 bool) (data string, md5 string) {
	routingRuleLength := len(input.RoutingRules)
	xml := make([]string, 0, 6+routingRuleLength*10)
	xml = append(xml, "<WebsiteConfiguration>")

	if input.RedirectAllRequestsTo.HostName != "" {
		xml = append(xml, fmt.Sprintf("<RedirectAllRequestsTo><HostName>%s</HostName>", input.RedirectAllRequestsTo.HostName))
		if input.RedirectAllRequestsTo.Protocol != "" {
			xml = append(xml, fmt.Sprintf("<Protocol>%s</Protocol>", input.RedirectAllRequestsTo.Protocol))
		}
		xml = append(xml, "</RedirectAllRequestsTo>")
	} else {
		if input.IndexDocument.Suffix != "" {
			indexDocumentSuffix := XmlTranscoding(input.IndexDocument.Suffix)
			xml = append(xml, fmt.Sprintf("<IndexDocument><Suffix>%s</Suffix></IndexDocument>", indexDocumentSuffix))
		}
		if input.ErrorDocument.Key != "" {
			errorDocumentKey := XmlTranscoding(input.ErrorDocument.Key)
			xml = append(xml, fmt.Sprintf("<ErrorDocument><Key>%s</Key></ErrorDocument>", errorDocumentKey))
		}
		if routingRuleLength > 0 {
			xml = append(xml, "<RoutingRules>")
			xml = append(xml, prepareRoutingRule(input))
			xml = append(xml, "</RoutingRules>")
		}
	}

	xml = append(xml, "</WebsiteConfiguration>")
	data = strings.Join(xml, "")
	if returnMd5 {
		md5 = Base64Md5([]byte(data))
	}
	return
}

func convertTransitionsToXML(transitions []Transition, isObs bool) string {
	if length := len(transitions); length > 0 {
		xml := make([]string, 0, length)
		for _, transition := range transitions {
			var temp string
			if transition.Days > 0 {
				temp = fmt.Sprintf("<Days>%d</Days>", transition.Days)
			} else if !transition.Date.IsZero() {
				temp = fmt.Sprintf("<Date>%s</Date>", transition.Date.UTC().Format(ISO8601_MIDNIGHT_DATE_FORMAT))
			}
			if temp != "" {
				if !isObs {
					storageClass := string(transition.StorageClass)
					if transition.StorageClass == StorageClassWarm {
						storageClass = string(storageClassStandardIA)
					} else if transition.StorageClass == StorageClassCold {
						storageClass = string(storageClassGlacier)
					}
					xml = append(xml, fmt.Sprintf("<Transition>%s<StorageClass>%s</StorageClass></Transition>", temp, storageClass))
				} else {
					xml = append(xml, fmt.Sprintf("<Transition>%s<StorageClass>%s</StorageClass></Transition>", temp, transition.StorageClass))
				}
			}
		}
		return strings.Join(xml, "")
	}
	return ""
}

func convertExpirationToXML(expiration Expiration) string {
	if expiration.Days > 0 {
		return fmt.Sprintf("<Expiration><Days>%d</Days></Expiration>", expiration.Days)
	} else if !expiration.Date.IsZero() {
		return fmt.Sprintf("<Expiration><Date>%s</Date></Expiration>", expiration.Date.UTC().Format(ISO8601_MIDNIGHT_DATE_FORMAT))
	}
	return ""
}
func convertNoncurrentVersionTransitionsToXML(noncurrentVersionTransitions []NoncurrentVersionTransition, isObs bool) string {
	if length := len(noncurrentVersionTransitions); length > 0 {
		xml := make([]string, 0, length)
		for _, noncurrentVersionTransition := range noncurrentVersionTransitions {
			if noncurrentVersionTransition.NoncurrentDays > 0 {
				storageClass := string(noncurrentVersionTransition.StorageClass)
				if !isObs {
					if storageClass == string(StorageClassWarm) {
						storageClass = string(storageClassStandardIA)
					} else if storageClass == string(StorageClassCold) {
						storageClass = string(storageClassGlacier)
					}
				}
				xml = append(xml, fmt.Sprintf("<NoncurrentVersionTransition><NoncurrentDays>%d</NoncurrentDays>"+
					"<StorageClass>%s</StorageClass></NoncurrentVersionTransition>",
					noncurrentVersionTransition.NoncurrentDays, storageClass))
			}
		}
		return strings.Join(xml, "")
	}
	return ""
}
func convertNoncurrentVersionExpirationToXML(noncurrentVersionExpiration NoncurrentVersionExpiration) string {
	if noncurrentVersionExpiration.NoncurrentDays > 0 {
		return fmt.Sprintf("<NoncurrentVersionExpiration><NoncurrentDays>%d</NoncurrentDays></NoncurrentVersionExpiration>", noncurrentVersionExpiration.NoncurrentDays)
	}
	return ""
}

// ConvertLifecyleConfigurationToXml converts BucketLifecyleConfiguration value to XML data and returns it
func ConvertLifecyleConfigurationToXml(input BucketLifecyleConfiguration, returnMd5 bool, isObs bool) (data string, md5 string) {
	xml := make([]string, 0, 2+len(input.LifecycleRules)*9)
	xml = append(xml, "<LifecycleConfiguration>")
	for _, lifecyleRule := range input.LifecycleRules {
		xml = append(xml, "<Rule>")
		if lifecyleRule.ID != "" {
			lifecyleRuleID := XmlTranscoding(lifecyleRule.ID)
			xml = append(xml, fmt.Sprintf("<ID>%s</ID>", lifecyleRuleID))
		}
		lifecyleRulePrefix := XmlTranscoding(lifecyleRule.Prefix)
		xml = append(xml, fmt.Sprintf("<Prefix>%s</Prefix>", lifecyleRulePrefix))
		xml = append(xml, fmt.Sprintf("<Status>%s</Status>", lifecyleRule.Status))
		if ret := convertTransitionsToXML(lifecyleRule.Transitions, isObs); ret != "" {
			xml = append(xml, ret)
		}
		if ret := convertExpirationToXML(lifecyleRule.Expiration); ret != "" {
			xml = append(xml, ret)
		}
		if ret := convertNoncurrentVersionTransitionsToXML(lifecyleRule.NoncurrentVersionTransitions, isObs); ret != "" {
			xml = append(xml, ret)
		}
		if ret := convertNoncurrentVersionExpirationToXML(lifecyleRule.NoncurrentVersionExpiration); ret != "" {
			xml = append(xml, ret)
		}
		xml = append(xml, "</Rule>")
	}
	xml = append(xml, "</LifecycleConfiguration>")
	data = strings.Join(xml, "")
	if returnMd5 {
		md5 = Base64Md5([]byte(data))
	}
	return
}

// ConvertEncryptionConfigurationToXml converts BucketEncryptionConfiguration value to XML data and returns it
func ConvertEncryptionConfigurationToXml(input BucketEncryptionConfiguration, returnMd5 bool, isObs bool) (data string, md5 string) {
	xml := make([]string, 0, 5)
	xml = append(xml, "<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault>")

	algorithm := XmlTranscoding(input.SSEAlgorithm)
	xml = append(xml, fmt.Sprintf("<SSEAlgorithm>%s</SSEAlgorithm>", algorithm))

	if input.KMSMasterKeyID != "" {
		kmsKeyID := XmlTranscoding(input.KMSMasterKeyID)
		xml = append(xml, fmt.Sprintf("<KMSMasterKeyID>%s</KMSMasterKeyID>", kmsKeyID))
	}
	if input.ProjectID != "" {
		projectID := XmlTranscoding(input.ProjectID)
		xml = append(xml, fmt.Sprintf("<ProjectID>%s</ProjectID>", projectID))
	}

	xml = append(xml, "</ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>")
	data = strings.Join(xml, "")
	if returnMd5 {
		md5 = Base64Md5([]byte(data))
	}
	return
}

func converntFilterRulesToXML(filterRules []FilterRule, isObs bool) string {
	if length := len(filterRules); length > 0 {
		xml := make([]string, 0, length*4)
		for _, filterRule := range filterRules {
			xml = append(xml, "<FilterRule>")
			if filterRule.Name != "" {
				filterRuleName := XmlTranscoding(filterRule.Name)
				xml = append(xml, fmt.Sprintf("<Name>%s</Name>", filterRuleName))
			}
			if filterRule.Value != "" {
				filterRuleValue := XmlTranscoding(filterRule.Value)
				xml = append(xml, fmt.Sprintf("<Value>%s</Value>", filterRuleValue))
			}
			xml = append(xml, "</FilterRule>")
		}
		if !isObs {
			return fmt.Sprintf("<Filter><S3Key>%s</S3Key></Filter>", strings.Join(xml, ""))
		}
		return fmt.Sprintf("<Filter><Object>%s</Object></Filter>", strings.Join(xml, ""))
	}
	return ""
}

func converntEventsToXML(events []EventType, isObs bool) string {
	if length := len(events); length > 0 {
		xml := make([]string, 0, length)
		if !isObs {
			for _, event := range events {
				xml = append(xml, fmt.Sprintf("<Event>%s%s</Event>", "s3:", event))
			}
		} else {
			for _, event := range events {
				xml = append(xml, fmt.Sprintf("<Event>%s</Event>", event))
			}
		}
		return strings.Join(xml, "")
	}
	return ""
}

func converntConfigureToXML(topicConfiguration TopicConfiguration, xmlElem string, isObs bool) string {
	xml := make([]string, 0, 6)
	xml = append(xml, xmlElem)
	if topicConfiguration.ID != "" {
		topicConfigurationID := XmlTranscoding(topicConfiguration.ID)
		xml = append(xml, fmt.Sprintf("<Id>%s</Id>", topicConfigurationID))
	}
	topicConfigurationTopic := XmlTranscoding(topicConfiguration.Topic)
	xml = append(xml, fmt.Sprintf("<Topic>%s</Topic>", topicConfigurationTopic))

	if ret := converntEventsToXML(topicConfiguration.Events, isObs); ret != "" {
		xml = append(xml, ret)
	}
	if ret := converntFilterRulesToXML(topicConfiguration.FilterRules, isObs); ret != "" {
		xml = append(xml, ret)
	}
	tempElem := xmlElem[0:1] + "/" + xmlElem[1:]
	xml = append(xml, tempElem)
	return strings.Join(xml, "")
}

// ConverntObsRestoreToXml converts RestoreObjectInput value to XML data and returns it
func ConverntObsRestoreToXml(restoreObjectInput RestoreObjectInput) string {
	xml := make([]string, 0, 2)
	xml = append(xml, fmt.Sprintf("<RestoreRequest><Days>%d</Days>", restoreObjectInput.Days))
	if restoreObjectInput.Tier != "Bulk" {
		xml = append(xml, fmt.Sprintf("<RestoreJob><Tier>%s</Tier></RestoreJob>", restoreObjectInput.Tier))
	}
	xml = append(xml, fmt.Sprintf("</RestoreRequest>"))
	data := strings.Join(xml, "")
	return data
}

// ConvertNotificationToXml converts BucketNotification value to XML data and returns it
func ConvertNotificationToXml(input BucketNotification, returnMd5 bool, isObs bool) (data string, md5 string) {
	xml := make([]string, 0, 2+len(input.TopicConfigurations)*6)
	xml = append(xml, "<NotificationConfiguration>")
	for _, topicConfiguration := range input.TopicConfigurations {
		ret := converntConfigureToXML(topicConfiguration, "<TopicConfiguration>", isObs)
		xml = append(xml, ret)
	}
	xml = append(xml, "</NotificationConfiguration>")
	data = strings.Join(xml, "")
	if returnMd5 {
		md5 = Base64Md5([]byte(data))
	}
	return
}

// ConvertCompleteMultipartUploadInputToXml converts CompleteMultipartUploadInput value to XML data and returns it
func ConvertCompleteMultipartUploadInputToXml(input CompleteMultipartUploadInput, returnMd5 bool) (data string, md5 string) {
	xml := make([]string, 0, 2+len(input.Parts)*4)
	xml = append(xml, "<CompleteMultipartUpload>")
	for _, part := range input.Parts {
		xml = append(xml, "<Part>")
		xml = append(xml, fmt.Sprintf("<PartNumber>%d</PartNumber>", part.PartNumber))
		xml = append(xml, fmt.Sprintf("<ETag>%s</ETag>", part.ETag))
		xml = append(xml, "</Part>")
	}
	xml = append(xml, "</CompleteMultipartUpload>")
	data = strings.Join(xml, "")
	if returnMd5 {
		md5 = Base64Md5([]byte(data))
	}
	return
}

func convertDeleteObjectsToXML(input DeleteObjectsInput) (data string, md5 string) {
	xml := make([]string, 0, 4+len(input.Objects)*4)
	xml = append(xml, "<Delete>")
	if input.Quiet {
		xml = append(xml, fmt.Sprintf("<Quiet>%t</Quiet>", input.Quiet))
	}
	if input.EncodingType != "" {
		encodingType := XmlTranscoding(input.EncodingType)
		xml = append(xml, fmt.Sprintf("<EncodingType>%s</EncodingType>", encodingType))
	}
	for _, obj := range input.Objects {
		xml = append(xml, "<Object>")
		key := XmlTranscoding(obj.Key)
		xml = append(xml, fmt.Sprintf("<Key>%s</Key>", key))
		if obj.VersionId != "" {
			xml = append(xml, fmt.Sprintf("<VersionId>%s</VersionId>", obj.VersionId))
		}
		xml = append(xml, "</Object>")
	}
	xml = append(xml, "</Delete>")
	data = strings.Join(xml, "")
	md5 = Base64Md5([]byte(data))
	return
}

func parseSseHeader(responseHeaders map[string][]string) (sseHeader ISseHeader) {
	if ret, ok := responseHeaders[HEADER_SSEC_ENCRYPTION]; ok {
		sseCHeader := SseCHeader{Encryption: ret[0]}
		if ret, ok = responseHeaders[HEADER_SSEC_KEY_MD5]; ok {
			sseCHeader.KeyMD5 = ret[0]
		}
		sseHeader = sseCHeader
	} else if ret, ok := responseHeaders[HEADER_SSEKMS_ENCRYPTION]; ok {
		sseKmsHeader := SseKmsHeader{Encryption: ret[0]}
		if ret, ok = responseHeaders[HEADER_SSEKMS_KEY]; ok {
			sseKmsHeader.Key = ret[0]
		} else if ret, ok = responseHeaders[HEADER_SSEKMS_ENCRYPT_KEY_OBS]; ok {
			sseKmsHeader.Key = ret[0]
		}
		sseHeader = sseKmsHeader
	}
	return
}

func parseCorsHeader(output BaseModel) (AllowOrigin, AllowHeader, AllowMethod, ExposeHeader string, MaxAgeSeconds int) {
	if ret, ok := output.ResponseHeaders[HEADER_ACCESS_CONRTOL_ALLOW_ORIGIN]; ok {
		AllowOrigin = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_ACCESS_CONRTOL_ALLOW_HEADERS]; ok {
		AllowHeader = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_ACCESS_CONRTOL_MAX_AGE]; ok {
		MaxAgeSeconds = StringToInt(ret[0], 0)
	}
	if ret, ok := output.ResponseHeaders[HEADER_ACCESS_CONRTOL_ALLOW_METHODS]; ok {
		AllowMethod = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_ACCESS_CONRTOL_EXPOSE_HEADERS]; ok {
		ExposeHeader = ret[0]
	}
	return
}

func parseUnCommonHeader(output *GetObjectMetadataOutput) {
	if ret, ok := output.ResponseHeaders[HEADER_VERSION_ID]; ok {
		output.VersionId = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_WEBSITE_REDIRECT_LOCATION]; ok {
		output.WebsiteRedirectLocation = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_EXPIRATION]; ok {
		output.Expiration = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_RESTORE]; ok {
		output.Restore = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_OBJECT_TYPE]; ok {
		output.ObjectType = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_NEXT_APPEND_POSITION]; ok {
		output.NextAppendPosition = ret[0]
	}
}

func parseStandardMetadataHeader(output *GetObjectMetadataOutput) {
	httpHeader := HttpHeader{}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_TYPE]; ok {
		httpHeader.ContentType = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_ENCODING]; ok {
		httpHeader.ContentEncoding = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CACHE_CONTROL]; ok {
		httpHeader.CacheControl = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_DISPOSITION]; ok {
		httpHeader.ContentDisposition = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_LANGUAGE]; ok {
		httpHeader.ContentLanguage = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_EXPIRES]; ok {
		httpHeader.HttpExpires = ret[0]
	}
	output.HttpHeader = httpHeader
}

// ParseGetObjectMetadataOutput sets GetObjectMetadataOutput field values with response headers
func ParseGetObjectMetadataOutput(output *GetObjectMetadataOutput) {
	output.AllowOrigin, output.AllowHeader, output.AllowMethod, output.ExposeHeader, output.MaxAgeSeconds = parseCorsHeader(output.BaseModel)
	parseUnCommonHeader(output)
	parseStandardMetadataHeader(output)
	if ret, ok := output.ResponseHeaders[HEADER_STORAGE_CLASS2]; ok {
		output.StorageClass = ParseStringToStorageClassType(ret[0])
	}
	if ret, ok := output.ResponseHeaders[HEADER_ETAG]; ok {
		output.ETag = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_TYPE]; ok {
		output.ContentType = ret[0]
	}

	output.SseHeader = parseSseHeader(output.ResponseHeaders)
	if ret, ok := output.ResponseHeaders[HEADER_LASTMODIFIED]; ok {
		ret, err := time.Parse(time.RFC1123, ret[0])
		if err == nil {
			output.LastModified = ret
		}
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_LENGTH]; ok {
		output.ContentLength = StringToInt64(ret[0], 0)
	}

	output.Metadata = make(map[string]string)

	for key, value := range output.ResponseHeaders {
		if strings.HasPrefix(key, PREFIX_META) {
			_key := key[len(PREFIX_META):]
			output.ResponseHeaders[_key] = value
			output.Metadata[_key] = value[0]
			delete(output.ResponseHeaders, key)
		}
	}

}

// ParseCopyObjectOutput sets CopyObjectOutput field values with response headers
func ParseCopyObjectOutput(output *CopyObjectOutput) {
	if ret, ok := output.ResponseHeaders[HEADER_VERSION_ID]; ok {
		output.VersionId = ret[0]
	}
	output.SseHeader = parseSseHeader(output.ResponseHeaders)
	if ret, ok := output.ResponseHeaders[HEADER_COPY_SOURCE_VERSION_ID]; ok {
		output.CopySourceVersionId = ret[0]
	}
}

// ParsePutObjectOutput sets PutObjectOutput field values with response headers
func ParsePutObjectOutput(output *PutObjectOutput) {
	if ret, ok := output.ResponseHeaders[HEADER_VERSION_ID]; ok {
		output.VersionId = ret[0]
	}
	output.SseHeader = parseSseHeader(output.ResponseHeaders)
	if ret, ok := output.ResponseHeaders[HEADER_STORAGE_CLASS2]; ok {
		output.StorageClass = ParseStringToStorageClassType(ret[0])
	}
	if ret, ok := output.ResponseHeaders[HEADER_ETAG]; ok {
		output.ETag = ret[0]
	}
}

// ParseInitiateMultipartUploadOutput sets InitiateMultipartUploadOutput field values with response headers
func ParseInitiateMultipartUploadOutput(output *InitiateMultipartUploadOutput) {
	output.SseHeader = parseSseHeader(output.ResponseHeaders)
}

// ParseUploadPartOutput sets UploadPartOutput field values with response headers
func ParseUploadPartOutput(output *UploadPartOutput) {
	output.SseHeader = parseSseHeader(output.ResponseHeaders)
	if ret, ok := output.ResponseHeaders[HEADER_ETAG]; ok {
		output.ETag = ret[0]
	}
}

// ParseCompleteMultipartUploadOutput sets CompleteMultipartUploadOutput field values with response headers
func ParseCompleteMultipartUploadOutput(output *CompleteMultipartUploadOutput) {
	output.SseHeader = parseSseHeader(output.ResponseHeaders)
	if ret, ok := output.ResponseHeaders[HEADER_VERSION_ID]; ok {
		output.VersionId = ret[0]
	}
}

// ParseCopyPartOutput sets CopyPartOutput field values with response headers
func ParseCopyPartOutput(output *CopyPartOutput) {
	output.SseHeader = parseSseHeader(output.ResponseHeaders)
}

// ParseGetBucketMetadataOutput sets GetBucketMetadataOutput field values with response headers
func ParseGetBucketMetadataOutput(output *GetBucketMetadataOutput) {
	output.AllowOrigin, output.AllowHeader, output.AllowMethod, output.ExposeHeader, output.MaxAgeSeconds = parseCorsHeader(output.BaseModel)
	if ret, ok := output.ResponseHeaders[HEADER_STORAGE_CLASS]; ok {
		output.StorageClass = ParseStringToStorageClassType(ret[0])
	} else if ret, ok := output.ResponseHeaders[HEADER_STORAGE_CLASS2]; ok {
		output.StorageClass = ParseStringToStorageClassType(ret[0])
	}
	if ret, ok := output.ResponseHeaders[HEADER_VERSION_OBS]; ok {
		output.Version = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_BUCKET_REGION]; ok {
		output.Location = ret[0]
	} else if ret, ok := output.ResponseHeaders[HEADER_BUCKET_LOCATION_OBS]; ok {
		output.Location = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_EPID_HEADERS]; ok {
		output.Epid = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_AZ_REDUNDANCY]; ok {
		output.AZRedundancy = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_BUCKET_REDUNDANCY]; ok {
		output.BucketRedundancy = parseStringToBucketRedundancy(ret[0])
	}
	if ret, ok := output.ResponseHeaders[headerFSFileInterface]; ok {
		output.FSStatus = parseStringToFSStatusType(ret[0])
	} else {
		output.FSStatus = FSStatusDisabled
	}
}

func parseContentHeader(output *SetObjectMetadataOutput) {
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_DISPOSITION]; ok {
		output.ContentDisposition = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_ENCODING]; ok {
		output.ContentEncoding = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_LANGUAGE]; ok {
		output.ContentLanguage = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_TYPE]; ok {
		output.ContentType = ret[0]
	}
}

// ParseSetObjectMetadataOutput sets SetObjectMetadataOutput field values with response headers
func ParseSetObjectMetadataOutput(output *SetObjectMetadataOutput) {
	if ret, ok := output.ResponseHeaders[HEADER_STORAGE_CLASS]; ok {
		output.StorageClass = ParseStringToStorageClassType(ret[0])
	} else if ret, ok := output.ResponseHeaders[HEADER_STORAGE_CLASS2]; ok {
		output.StorageClass = ParseStringToStorageClassType(ret[0])
	}
	if ret, ok := output.ResponseHeaders[HEADER_METADATA_DIRECTIVE]; ok {
		output.MetadataDirective = MetadataDirectiveType(ret[0])
	}
	if ret, ok := output.ResponseHeaders[HEADER_CACHE_CONTROL]; ok {
		output.CacheControl = ret[0]
	}
	parseContentHeader(output)
	if ret, ok := output.ResponseHeaders[HEADER_EXPIRES]; ok {
		output.Expires = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_WEBSITE_REDIRECT_LOCATION]; ok {
		output.WebsiteRedirectLocation = ret[0]
	}
	output.Metadata = make(map[string]string)

	for key, value := range output.ResponseHeaders {
		if strings.HasPrefix(key, PREFIX_META) {
			_key := key[len(PREFIX_META):]
			output.ResponseHeaders[_key] = value
			output.Metadata[_key] = value[0]
			delete(output.ResponseHeaders, key)
		}
	}
}

// ParseDeleteObjectOutput sets DeleteObjectOutput field values with response headers
func ParseDeleteObjectOutput(output *DeleteObjectOutput) {
	if versionID, ok := output.ResponseHeaders[HEADER_VERSION_ID]; ok {
		output.VersionId = versionID[0]
	}

	if deleteMarker, ok := output.ResponseHeaders[HEADER_DELETE_MARKER]; ok {
		output.DeleteMarker = deleteMarker[0] == "true"
	}
}

// ParseGetObjectOutput sets GetObjectOutput field values with response headers
func ParseGetObjectOutput(output *GetObjectOutput) {
	ParseGetObjectMetadataOutput(&output.GetObjectMetadataOutput)
	if ret, ok := output.ResponseHeaders[HEADER_DELETE_MARKER]; ok {
		output.DeleteMarker = ret[0] == "true"
	}
	if ret, ok := output.ResponseHeaders[HEADER_CACHE_CONTROL]; ok {
		output.CacheControl = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_DISPOSITION]; ok {
		output.ContentDisposition = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_ENCODING]; ok {
		output.ContentEncoding = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_CONTENT_LANGUAGE]; ok {
		output.ContentLanguage = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_EXPIRES]; ok {
		output.Expires = ret[0]
	}
}

// ConvertRequestToIoReaderV2 converts req to XML data
func ConvertRequestToIoReaderV2(req interface{}) (io.Reader, string, error) {
	data, err := TransToXml(req)
	if err == nil {
		if isDebugLogEnabled() {
			doLog(LEVEL_DEBUG, "Do http request with data: %s", string(data))
		}
		return bytes.NewReader(data), Base64Md5(data), nil
	}
	return nil, "", err
}

// ConvertRequestToIoReader converts req to XML data
func ConvertRequestToIoReader(req interface{}) (io.Reader, error) {
	body, err := TransToXml(req)
	if err == nil {
		if isDebugLogEnabled() {
			doLog(LEVEL_DEBUG, "Do http request with data: %s", string(body))
		}
		return bytes.NewReader(body), nil
	}
	return nil, err
}

func parseResponseBodyOutput(s reflect.Type, baseModel IBaseModel, body []byte) {
	for i := 0; i < s.NumField(); i++ {
		if s.Field(i).Tag == "json:\"body\"" {
			reflect.ValueOf(baseModel).Elem().FieldByName(s.Field(i).Name).SetString(string(body))
			break
		}
	}
}

// ParseCallbackResponseToBaseModel gets response from Callback Service
func ParseCallbackResponseToBaseModel(resp *http.Response, baseModel IBaseModel, isObs bool) error {
	baseModel.setStatusCode(resp.StatusCode)
	responseHeaders := cleanHeaderPrefix(resp.Header)
	baseModel.setResponseHeaders(responseHeaders)
	if values, ok := responseHeaders[HEADER_REQUEST_ID]; ok {
		baseModel.setRequestID(values[0])
	}
	readCloser, ok := baseModel.(ICallbackReadCloser)
	if !ok {
		return errors.New("Failed to set CallbackBody with resp's body.")
	}
	readCloser.setCallbackReadCloser(resp.Body)
	return nil
}

// ParseResponseToBaseModel gets response from OBS
func ParseResponseToBaseModel(resp *http.Response, baseModel IBaseModel, xmlResult bool, isObs bool) (err error) {
	readCloser, ok := baseModel.(IReadCloser)
	if !ok {
		defer func() {
			errMsg := resp.Body.Close()
			if errMsg != nil {
				doLog(LEVEL_WARN, "Failed to close response body")
			}
		}()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil && len(body) > 0 {
			if xmlResult {
				err = ParseXml(body, baseModel)
			} else {
				s := reflect.TypeOf(baseModel).Elem()
				name := reflect.TypeOf(baseModel).Elem().Name()
				if name == "GetBucketPolicyOutput" || name == "GetBucketMirrorBackToSourceOuput" {
					parseResponseBodyOutput(s, baseModel, body)
				} else {
					err = parseJSON(body, baseModel)
				}
			}
			if err != nil {
				doLog(LEVEL_ERROR, "Unmarshal error: %v", err)
			}
		}
	} else {
		readCloser.setReadCloser(resp.Body)
	}

	baseModel.setStatusCode(resp.StatusCode)
	responseHeaders := cleanHeaderPrefix(resp.Header)
	baseModel.setResponseHeaders(responseHeaders)
	if values, ok := responseHeaders[HEADER_REQUEST_ID]; ok {
		baseModel.setRequestID(values[0])
	}
	return
}

// ParseResponseToObsError gets obsError from OBS
func ParseResponseToObsError(resp *http.Response, isObs bool) error {
	isJson := false
	if contentType, ok := resp.Header[HEADER_CONTENT_TYPE_CAML]; ok {
		jsonType, _ := mimeTypes["json"]
		isJson = contentType[0] == jsonType
	}
	obsError := ObsError{}
	respError := ParseResponseToBaseModel(resp, &obsError, !isJson, isObs)
	if respError != nil {
		doLog(LEVEL_WARN, "Parse response to BaseModel with error: %v", respError)
	}
	obsError.Status = resp.Status
	return obsError
}

// convertFetchPolicyToJSON converts SetBucketFetchPolicyInput into json format
func convertFetchPolicyToJSON(input SetBucketFetchPolicyInput) (data string, err error) {
	fetch := map[string]SetBucketFetchPolicyInput{"fetch": input}
	json, err := json.Marshal(fetch)
	if err != nil {
		return "", err
	}
	data = string(json)
	return
}

// convertFetchJobToJSON converts SetBucketFetchJobInput into json format
func convertFetchJobToJSON(input SetBucketFetchJobInput) (data string, err error) {
	objectHeaders := make(map[string]string)
	for key, value := range input.ObjectHeaders {
		if value != "" {
			_key := strings.ToLower(key)
			if !strings.HasPrefix(key, HEADER_PREFIX_OBS) {
				_key = HEADER_PREFIX_META_OBS + _key
			}
			objectHeaders[_key] = value
		}
	}
	input.ObjectHeaders = objectHeaders
	json, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	data = string(json)
	return
}

func parseStringToFSStatusType(value string) (ret FSStatusType) {
	switch value {
	case "Enabled":
		ret = FSStatusEnabled
	case "Disabled":
		ret = FSStatusDisabled
	default:
		ret = ""
	}
	return
}

func parseStringToBucketRedundancy(value string) (ret BucketRedundancyType) {
	switch value {
	case "FUSION":
		ret = BucketRedundancyFusion
	case "CLASSIC":
		ret = BucketRedundancyClassic
	default:
		ret = ""
	}
	return
}

func decodeListObjectsOutput(output *ListObjectsOutput) (err error) {
	output.Delimiter, err = url.QueryUnescape(output.Delimiter)
	if err != nil {
		return
	}
	output.Marker, err = url.QueryUnescape(output.Marker)
	if err != nil {
		return
	}
	output.NextMarker, err = url.QueryUnescape(output.NextMarker)
	if err != nil {
		return
	}
	output.Prefix, err = url.QueryUnescape(output.Prefix)
	if err != nil {
		return
	}
	for index, value := range output.CommonPrefixes {
		output.CommonPrefixes[index], err = url.QueryUnescape(value)
		if err != nil {
			return
		}
	}
	for index, content := range output.Contents {
		output.Contents[index].Key, err = url.QueryUnescape(content.Key)
		if err != nil {
			return
		}
	}
	return
}

func decodeListVersionsOutput(output *ListVersionsOutput) (err error) {
	output.Delimiter, err = url.QueryUnescape(output.Delimiter)
	if err != nil {
		return
	}
	output.KeyMarker, err = url.QueryUnescape(output.KeyMarker)
	if err != nil {
		return
	}
	output.NextKeyMarker, err = url.QueryUnescape(output.NextKeyMarker)
	if err != nil {
		return
	}
	output.Prefix, err = url.QueryUnescape(output.Prefix)
	if err != nil {
		return
	}
	for index, version := range output.Versions {
		output.Versions[index].Key, err = url.QueryUnescape(version.Key)
		if err != nil {
			return
		}
	}
	for index, deleteMarker := range output.DeleteMarkers {
		output.DeleteMarkers[index].Key, err = url.QueryUnescape(deleteMarker.Key)
		if err != nil {
			return
		}
	}
	for index, value := range output.CommonPrefixes {
		output.CommonPrefixes[index], err = url.QueryUnescape(value)
		if err != nil {
			return
		}
	}
	return
}

func decodeDeleteObjectsOutput(output *DeleteObjectsOutput) (err error) {
	for index, object := range output.Deleteds {
		output.Deleteds[index].Key, err = url.QueryUnescape(object.Key)
		if err != nil {
			return
		}
	}
	for index, object := range output.Errors {
		output.Errors[index].Key, err = url.QueryUnescape(object.Key)
		if err != nil {
			return
		}
	}
	return
}

func decodeListMultipartUploadsOutput(output *ListMultipartUploadsOutput) (err error) {
	output.Delimiter, err = url.QueryUnescape(output.Delimiter)
	if err != nil {
		return
	}
	output.Prefix, err = url.QueryUnescape(output.Prefix)
	if err != nil {
		return
	}
	output.KeyMarker, err = url.QueryUnescape(output.KeyMarker)
	if err != nil {
		return
	}
	output.NextKeyMarker, err = url.QueryUnescape(output.NextKeyMarker)
	if err != nil {
		return
	}
	for index, value := range output.CommonPrefixes {
		output.CommonPrefixes[index], err = url.QueryUnescape(value)
		if err != nil {
			return
		}
	}
	for index, upload := range output.Uploads {
		output.Uploads[index].Key, err = url.QueryUnescape(upload.Key)
		if err != nil {
			return
		}
	}
	return
}

func decodeListPartsOutput(output *ListPartsOutput) (err error) {
	output.Key, err = url.QueryUnescape(output.Key)
	return
}

func decodeInitiateMultipartUploadOutput(output *InitiateMultipartUploadOutput) (err error) {
	output.Key, err = url.QueryUnescape(output.Key)
	return
}

func decodeCompleteMultipartUploadOutput(output *CompleteMultipartUploadOutput) (err error) {
	output.Key, err = url.QueryUnescape(output.Key)
	return
}

// ParseAppendObjectOutput sets AppendObjectOutput field values with response headers
func ParseAppendObjectOutput(output *AppendObjectOutput) (err error) {
	if ret, ok := output.ResponseHeaders[HEADER_VERSION_ID]; ok {
		output.VersionId = ret[0]
	}
	output.SseHeader = parseSseHeader(output.ResponseHeaders)
	if ret, ok := output.ResponseHeaders[HEADER_ETAG]; ok {
		output.ETag = ret[0]
	}
	if ret, ok := output.ResponseHeaders[HEADER_NEXT_APPEND_POSITION]; ok {
		output.NextAppendPosition, err = strconv.ParseInt(ret[0], 10, 64)
		if err != nil {
			err = fmt.Errorf("failed to parse next append position with error [%v]", err)
		}
	}
	return
}

// ParseModifyObjectOutput sets ModifyObjectOutput field values with response headers
func ParseModifyObjectOutput(output *ModifyObjectOutput) {
	if ret, ok := output.ResponseHeaders[HEADER_ETAG]; ok {
		output.ETag = ret[0]
	}
}

func ParseGetBucketFSStatusOutput(output *GetBucketFSStatusOutput) {
	ParseGetBucketMetadataOutput(&output.GetBucketMetadataOutput)

	if ret, ok := output.ResponseHeaders[HEADER_FS_FILE_INTERFACE_OBS]; ok {
		output.FSStatus = ParseStringToFSStatusType(ret[0])
	}
}

func ParseGetAttributeOutput(output *GetAttributeOutput) {
	ParseGetObjectMetadataOutput(&output.GetObjectMetadataOutput)
	if ret, ok := output.ResponseHeaders[HEADER_MODE]; ok {
		output.Mode = StringToInt(ret[0], -1)
	} else {
		output.Mode = -1
	}
}

func ParseNewFolderOutput(output *NewFolderOutput) {
	ParsePutObjectOutput(&output.PutObjectOutput)
}
