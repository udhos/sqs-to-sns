#!/bin/bash

# the ./docs dir is published as https://udhos.github.io/sqs-to-sns/

chart_dir=charts/sqs-to-sns
chart_url=https://udhos.github.io/sqs-to-sns/

docs_dir=../docs

# generate new chart package from source into ./docs
helm package $chart_dir -d $docs_dir

#
# copy new chart into ./charts-tmp
#

chart_name=$(gojq --yaml-input -r .name < $chart_dir/Chart.yaml)
chart_version=$(gojq --yaml-input -r .version < $chart_dir/Chart.yaml)
chart_pkg=${chart_name}-${chart_version}.tgz
rm -rf charts-tmp
mkdir -p charts-tmp
cp $docs_dir/${chart_pkg} charts-tmp

#
# merge new chart index into docs/index.yaml
#

git checkout $docs_dir/index.yaml ;# reset index

# regenerate the index from existing chart packages
helm repo index charts-tmp --url $chart_url --merge $docs_dir/index.yaml

# new merged chart index was generated as ./charts-tmp/index.yaml,
# copy it back to ./docs
cp charts-tmp/index.yaml $docs_dir

echo "#"
echo "# check that $docs_dir is fine then:"
echo "#"
echo "git add docs"
echo "git commit -m 'Update chart repository.'"
echo "git push"
