#!/usr/bin/env bash

set -e

wget https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-176.0.0-linux-x86_64.tar.gz

tar -xvzf google-cloud-sdk-176.0.0-linux-x86_64.tar.gz

./google-cloud-sdk/install.sh -q
