#!/bin/bash -e
#
# Assume the default is to use git to pull tf files and
# Assume the stack lives at the root of the untar-ed dir and
# Assume the tfvars lives at "tfvars" in root of untar-ed dir
TMP=`mktemp`
CLEAN=`mktemp`
REPO_COUNT=`find /tfops -type f  -not -path /tfops -name repo.tar|wc -l`
if [ "$REPO_COUNT" -gt 0 ];then
    find /tfops -type f  -not -path /tfops -name repo.tar -exec tar -xf {} -C / \;
else
    # Download the module and exectue from here
    `$DOWNLOAD_MAIN_MODULE`
fi

cd /$TFOPS_MAIN_MODULE
cp $TFOPS_STATE_FILE . || true 

terraform init .
terraform plan $TFOPS_VARFILE_FLAG -out plan.out .

# Apply? For now apply since there is no mechanism to decide this
terraform apply plan.out

# Deleting the state file is dangerous. Maybe this should be updated safely 
# with apply. Or maybe increase an index number appended to the cm name. This 
# could be useful to track deployment history.
kubectl delete cm ${INSTANCE_NAME}-tfstate || true
kubectl create cm ${INSTANCE_NAME}-tfstate --from-file terraform.tfstate

# Replan to see what tf thinks should happen next.
terraform plan $TFOPS_VARFILE_FLAG . 2>&1| tee $TMP
# Clean the output from coloration
cat $TMP | sed -r "s/\x1B\[([0-9]{1,2}(;[0-9]{1,2})?)?[mGK]//g" > $CLEAN

# Status Helpers:
awk '/^Error:/{y=1}y' $CLEAN > ERROR

read -d '' -r -a arr <<< `grep "^Plan:" $CLEAN|tr -dc '0-9,'|tr ',' ' '`
echo -n ${arr[0]} > PLAN
echo -n ${arr[1]} > CHANGE
echo -n ${arr[2]} > DESTROY

kubectl delete cm ${INSTANCE_NAME}-status || true
kubectl create cm ${INSTANCE_NAME}-status \
    --from-file ERROR \
    --from-file PLAN \
    --from-file CHANGE \
    --from-file DESTROY
