// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

// DatastoreProduct is used to identify your datastore type in New Relic.  It
// is used in the DatastoreSegment Product field.
type DatastoreProduct string

// Datastore names used across New Relic agents:
const (
	DatastoreCassandra     DatastoreProduct = "Cassandra"
	DatastoreCouchDB       DatastoreProduct = "CouchDB"
	DatastoreDerby         DatastoreProduct = "Derby"
	DatastoreDynamoDB      DatastoreProduct = "DynamoDB"
	DatastoreElasticsearch DatastoreProduct = "Elasticsearch"
	DatastoreFirebird      DatastoreProduct = "Firebird"
	DatastoreIBMDB2        DatastoreProduct = "IBMDB2"
	DatastoreInformix      DatastoreProduct = "Informix"
	DatastoreMemcached     DatastoreProduct = "Memcached"
	DatastoreMongoDB       DatastoreProduct = "MongoDB"
	DatastoreMSSQL         DatastoreProduct = "MSSQL"
	DatastoreMySQL         DatastoreProduct = "MySQL"
	DatastoreNeptune       DatastoreProduct = "Neptune"
	DatastoreOracle        DatastoreProduct = "Oracle"
	DatastorePostgres      DatastoreProduct = "Postgres"
	DatastoreRedis         DatastoreProduct = "Redis"
	DatastoreRiak          DatastoreProduct = "Riak"
	DatastoreSnowflake     DatastoreProduct = "Snowflake"
	DatastoreSolr          DatastoreProduct = "Solr"
	DatastoreSQLite        DatastoreProduct = "SQLite"
	DatastoreTarantool     DatastoreProduct = "Tarantool"
	DatastoreVoltDB        DatastoreProduct = "VoltDB"
	DatastoreAerospike     DatastoreProduct = "Aerospike"
)
