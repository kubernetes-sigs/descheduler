#!/usr/bin/env bash

set -e


gcloud auth activate-service-account --key-file "${GCE_SA_CREDS}"
gcloud config set project $GCE_PROJECT_ID
gcloud config set compute/zone $GCE_ZONE
