# Usage

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

    helm repo add sqs-to-sns https://udhos.github.io/sqs-to-sns

Update files from repo:

    helm repo update

Search sqs-to-sns:

    $ helm search repo sqs-to-sns -l --version ">=0.0.0"
    NAME                 	CHART VERSION	APP VERSION	DESCRIPTION
    sqs-to-sns/sqs-to-sns	0.3.0        	0.3.0      	A Helm chart for Kubernetes
    sqs-to-sns/sqs-to-sns	0.2.0        	0.2.0      	A Helm chart for Kubernetes
    sqs-to-sns/sqs-to-sns	0.1.0        	0.1.0      	A Helm chart for Kubernetes

To install the charts:

    helm install my-sqs-to-sns sqs-to-sns/sqs-to-sns
    #            ^             ^          ^
    #            |             |           \__________ chart
    #            |             |
    #            |              \_____________________ repo
    #            |
    #             \___________________________________ release (chart instance installed in cluster)

To uninstall the charts:

    helm uninstall my-sqs-to-sns

# Source

<https://github.com/udhos/sqs-to-sns>
