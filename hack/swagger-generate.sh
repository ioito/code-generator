#!/bin/bash

./_output/bin/swagger-gen \
    --input-dirs yunion.io/x/onecloud/pkg/compute/models \
    --input-dirs yunion.io/x/onecloud/pkg/image/models \
    --output-package yunion.io/x/onecloud/pkg/generated/swagger
