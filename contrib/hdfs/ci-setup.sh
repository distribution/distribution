#! /bin/bash
#
# HDFS cluster setup in Circle CI
#

set -x
set -e
set -u

# NODE=$(hostname)
# 
# echo $(ip route get 1 | awk '{print $NF;exit}') $(hostname) >> /etc/hosts
# ssh-keygen -t rsa -f ~/.ssh/id_rsa -q -N ""
# cat ~/.ssh/id_rsa.pub >> ~/.ssh/authorized_keys
# ssh-keyscan $NODE >> ~/.ssh/known_hosts

sudo -i apt-get update
sudo -i apt-get install openjdk-8-jre openjdk-8-jdk
export JAVA_HOME=/usr/lib/jvm/java-8-openjdk-amd64
wget https://archive.apache.org/dist/hadoop/core/hadoop-2.7.1/hadoop-2.7.1.tar.gz
sudo -i tar -zxf hadoop-2.7.1.tar.gz -C /usr/local
sudo -i mv /usr/local/hadoop-2.7.1 /usr/local/hadoop
export PATH=$PATH:/usr/local/hadoop/bin

cat > /usr/local/hadoop/etc/hadoop/core-site.xml <<EOF 
<?xml version="1.0" encoding="UTF-8"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<!--
  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License. See accompanying LICENSE file.
-->

<!-- Put site-specific property overrides in this file. -->

<configuration>
    <property>
        <name>hadoop.tmp.dir</name>
        <value>file:/usr/local/hadoop/tmp</value>
        <description>Abase for other temporary directories.</description>
    </property>
    <property>
        <name>fs.defaultFS</name>
        <value>hdfs://localhost:9000</value>
    </property>  
</configuration>
EOF

cat > /usr/local/hadoop/etc/hadoop/hdfs-site.xml <<EOF 
<?xml version="1.0" encoding="UTF-8"?>
<?xml-stylesheet type="text/xsl" href="configuration.xsl"?>
<!--
  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License. See accompanying LICENSE file.
-->

<!-- Put site-specific property overrides in this file. -->

<configuration>
    <property>
        <name>dfs.replication</name>
        <value>1</value>
    </property>
    <property>
        <name>dfs.namenode.name.dir</name>
        <value>file:/usr/local/hadoop/tmp/dfs/name</value>
    </property>
    <property>
        <name>dfs.datanode.data.dir</name>
        <value>file:/usr/local/hadoop/tmp/dfs/data</value>
    </property>
    <property>
        <name>dfs.client.block.write.replace-datanode-on-failure.enable</name>
        <value>false</value>
    </property>    
</configuration>
EOF

hdfs namenode -format
/usr/local/hadoop/sbin/start-dfs.sh

export HDFS_NAME_NODE_HOST=localhost
export HDFS_NAME_NODE_PORT=50070
