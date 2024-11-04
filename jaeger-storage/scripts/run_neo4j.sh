#!/bin/bash

echo "starting ne4j docker container..."

docker run \
-p 7474:7474 \
-p 7687:7687 \
-v $PWD/data:/data \
-v $PWD/plugins:/plugins \
-e NEO4J_apoc_export_file_enabled=true \
-e NEO4J_apoc_import_file_enabled=true \
-e NEO4J_apoc_import_file_use__neo4j__config=true \
-e NEO4J_dbms_security_procedures_unrestricted="apoc.*" \
-e NEO4J_PLUGINS=\["apoc","apoc-extended"\] \
-e NEO4J_AUTH=none \
neo4j:5.22
