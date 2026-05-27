package objects

import (
	miniogo "github.com/minio/minio-go/v7"
)

// entryFromObjectInfo maps minio-go's ObjectInfo (returned by
// ListObjectsV2 / StatObject) into the domain Entry value the read path
// exposes. The wire-level field set is broader than what the UI cares
// about; we deliberately ignore version IDs, owner metadata, replication
// status, etc. so the read view stays stable as MinIO adds new fields.
func entryFromObjectInfo(info miniogo.ObjectInfo) Entry {
	return Entry{
		Key:          info.Key,
		Size:         info.Size,
		LastModified: info.LastModified,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
	}
}

// entryFromUploadInfo maps minio-go's UploadInfo (returned by PutObject)
// into the domain Entry value the create path returns to the caller.
// ContentType is not surfaced on UploadInfo — the caller threads the
// request-supplied value through explicitly.
func entryFromUploadInfo(info miniogo.UploadInfo, contentType string) Entry {
	return Entry{
		Key:          info.Key,
		Size:         info.Size,
		LastModified: info.LastModified,
		ContentType:  contentType,
		ETag:         info.ETag,
	}
}

// prefixFromCommonPrefix maps minio-go's CommonPrefix into the domain
// Prefix value. A one-line wrapper today; the indirection lets a future
// task fold prefix-level statistics (object count, size estimate) in
// without changing call sites.
func prefixFromCommonPrefix(cp miniogo.CommonPrefix) Prefix {
	return Prefix{Name: cp.Prefix}
}
