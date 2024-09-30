#!/usr/bin/env sh
#-*-mode: Shell-script; coding: utf-8;-*-
# SPDX-License-Identifier: BlueOak-1.0.0
# Description:
_base=$(basename "$0")
_dir=$(cd -P -- "$(dirname -- "$(command -v -- "$0")")" && pwd -P || exit 126)
export _base _dir
set "${SETOPTS:--eu}"

# Have to expose/add 8082 to let the docker registry to work
kubectl -n nexus patch svc nexus-nexus-repository-manager -p '{"spec":{"ports":[{"name":"docker-port","port":8082,"targetPort":8082}]}}'

# As well as append the path to the ingress
kubectl -n nexus patch ingress nexus-nexus-repository-manager --type=json -p '[{"op":"add","path":"/spec/rules/0/http/paths/-","value":{"path":"/v2","pathType":"Prefix","backend":{"service":{"name":"nexus-nexus-repository-manager","port":{"number":8082}}}}}]'

# Since we're doing this all from not the gui, grab the generated password (no
# way to set it at install time on the old open source helm chart)
user="admin"
npod=$(kubectl get pods -n nexus --no-headers=true | awk '{print $1}')
pass=$(
  kubectl exec -n nexus "${npod}" -- cat /nexus-data/admin.password
  printf "\n"
)
host=nexus.dev.home.arpa
proto=http

nexushost="${proto}://${host}"
httpbasicauth="${user}:${pass}"

# Then before we can use any docker repo, we need to allow the docker repo to
# use token authentication and to setup nexus to also use authenticating realm
# for docker tokens.
curl -u "${httpbasicauth}" -X PUT -H 'accept: application/json' -H 'Content-Type: application/json' -i "${nexushost}/service/rest/v1/security/realms/active" -d '["NexusAuthenticatingRealm","DockerToken"]'

# OK after all that nonsense, we can create our docker repo in nexus.
curl -u "${httpbasicauth}" -X POST -H 'Content-Type: application/json' -i "${nexushost}/service/rest/v1/repositories/docker/hosted" -d '{
    "name": "docker-repo",
    "online": true,
    "storage": {
      "blobStoreName": "default",
      "strictContentTypeValidation": true,
      "writePolicy": "ALLOW"
    },
    "docker": {
      "v1Enabled": false,
      "forceBasicAuth": true,
      "httpPort": 8082,
      "httpsPort": 8083
    }
}'

# As well as the helm repo
curl -u "${httpbasicauth}" -X POST -H 'Content-Type: application/json' -i "${nexushost}/service/rest/v1/repositories/helm/hosted" -d '{
    "name": "helm-repo",
    "online": true,
    "storage": {
      "blobStoreName": "default",
      "strictContentTypeValidation": true,
      "writePolicy": "ALLOW"
    },
    "helm": {
      "chartNameValidation": true
    }
}'
