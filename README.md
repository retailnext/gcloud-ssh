# gcloud-ssh

This is a wrapper around ssh and scp to allow ansible and ansible tower/awx to launch jobs on managed instances using gcloud login with a service account with iam.serviceAccounts.actAs permission and with no need of installing private SSH keys on the managed instances.
