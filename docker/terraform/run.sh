#!/bin/bash -e
# Let the finalizer or manual destroy by passing in $DESTROY="true"
# Terraforms will be applyed only then $APPLY="true"
function run_terraform {
    # options: 
    #   destroy=true|false
    declare $@

    if [ "$destroy" = "true" ];then
        destroy_when_true='-destroy'
    fi  

    terraform init .
    if [ $? -gt 0 ];then return 1;fi

    terraform plan $TFOPS_VARFILE_FLAG $destroy_when_true -out plan.out . 2>&1| tee $TMP
    if [ ${PIPESTATUS[0]} -gt 0 ];then 
        set +x
        save_plan
        set -x
        return 1
    else
        set +x
        save_plan
        set -x
    fi
    set +x

    n=0
    while true;do
        (( n++ ))
        get_action # sets $action variable
        if [ "$action" = "apply" ];then
            break
        elif [ "$action" = "abort" ];then
            exit 0
        fi
        if [ $n -gt 60 ];then
            echo "Waiting for user action. Check the cm/${INSTANCE_NAME}-action"
            n=0
        fi
        sleep 1
    done

    set -x
    terraform apply plan.out
    if [ $? -gt 0 ];then return 1;fi
    
    set +x
    # Replan to see what tf thinks should happen next.
    terraform plan $TFOPS_VARFILE_FLAG $destroy_when_true . 2>&1| tee $TMP
}

function save_plan {
    # Clean the output from coloration
    cat $TMP | sed -r "s/\x1B\[([0-9]{1,2}(;[0-9]{1,2})?)?[mGK]//g" > $CLEAN

    # Save the intermediate plan
    cp $CLEAN tfplan
    kubectl delete cm ${INSTANCE_NAME}-plan > /dev/null 2>&1 || true
    kubectl create cm ${INSTANCE_NAME}-plan --from-file tfplan
}

function get_action {
    action=`kubectl get cm ${INSTANCE_NAME}-action -ojsonpath='{.data.action}'`
    export action
}

if [ "$GIT_PASSWORD" != "" ];then
    echo setting git password
    ASKPASS=`mktemp`
    cat << EOF > $ASKPASS
#!/bin/sh
exec echo "$GIT_PASSWORD"
EOF
    chmod +x $ASKPASS
    export GIT_ASKPASS=$ASKPASS
fi 

# Troubleshooting lines
# env
# ls -lah ~/.ssh

#
# Assume the default is to use git to pull tf files and
# Assume the tfvars lives at "tfvars" in root of untar-ed dir
export TMP=`mktemp`
export CLEAN=`mktemp`
REPO_COUNT=`find /tfops -type f  -not -path /tfops -name repo.tar|wc -l`


if [ "$STACK_REPO" != "" ];then
    set -x
    MAIN_MODULE_TMP=`mktemp -d`
    git clone $STACK_REPO $MAIN_MODULE_TMP/stack
    cd $MAIN_MODULE_TMP/stack
    git checkout $STACK_REPO_HASH
    if [ "$STACK_REPO_SUBDIR" != "" ];then
        pwd
        ls -lah
        cp -r $STACK_REPO_SUBDIR /$TFOPS_MAIN_MODULE
    else
        mv $MAIN_MODULE_TMP/stack /$TFOPS_MAIN_MODULE
    fi
elif [ "$REPO_COUNT" -gt 0 ];then
        find /tfops -type f  -not -path /tfops -name repo.tar -exec tar -xf {} -C / \;
else
    echo "No terraform stack to run"
    exit 1
fi

if [ "$TFOPS_CONFIGMAP_PATH" != "" ];then
    cp $TFOPS_CONFIGMAP_PATH/* /$TFOPS_MAIN_MODULE
fi

cd /$TFOPS_MAIN_MODULE

# Load a custom backend
set -x
envsubst < /backend.tf > /backend_override.tf
mv /backend_override.tf .

WAIT_TIME=${WAIT_TIME:-60}
ATTEMPTS=${ATTEMPTS:-10}
i=0
until run_terraform destroy=$DESTROY || (( i++ >= $ATTEMPTS ));do
    echo "($i/$ATTEMPTS) Terraform did not exit 0, waiting $WAIT_TIME"
    sleep $WAIT_TIME
done

# Clean the output from coloration
cat $TMP | sed -r "s/\x1B\[([0-9]{1,2}(;[0-9]{1,2})?)?[mGK]//g" > $CLEAN

# Status Helpers:
awk '/^Error:/{y=1}y' $CLEAN > ERROR

read -d '' -r -a arr <<< `grep "^Plan:" $CLEAN|tr -dc '0-9,'|tr ',' ' '`
echo -n ${arr[0]} > PLAN
echo -n ${arr[1]} > CHANGE
echo -n ${arr[2]} > DESTROY

kubectl delete cm ${INSTANCE_NAME}-status > /dev/null 2>&1 || true
kubectl create cm ${INSTANCE_NAME}-status \
    --from-file ERROR \
    --from-file PLAN \
    --from-file CHANGE \
    --from-file DESTROY


save_plan