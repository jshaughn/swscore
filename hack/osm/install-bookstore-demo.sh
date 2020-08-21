#!/bin/bash

##############################################################################
# install-bookstore-demo.sh
#
# Installs the OSM Bookstore Example Demo Application into your cluster
# (either Kubernetes or OpenShift).
#
# See --help for more details on options to this script.
#
##############################################################################

# OSM_DIR is where the OSM is cloned and thus where the demo files are found.
# CLIENT_EXE_NAME is going to either be "oc" or "kubectl"
OSM_DIR=
CLIENT_EXE_NAME="oc"
NAMESPACES="bookstore bookbuyer bookwarehouse bookthief"
OSM_NAMESPACE="osm-system"
RATE=1
AUTO_INJECTION="true"
DELETE_BOOKSTORE="false"
STATUS"false"

# process command line args
while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    -ai|--auto-injection)
      AUTO_INJECTION="$2"
      shift;shift
      ;;
    -db|--delete-bookstore)
      DELETE_BOOKSTORE="$2"
      shift;shift
      ;;
    -id|--osm-dir)
      OSM_DIR="$2"
      shift;shift
      ;;
    -in|--istio-namespace)
      OSM_NAMESPACE="$2"
      shift;shift
      ;;
    -c|--client-exe)
      CLIENT_EXE_NAME="$2"
      shift;shift
      ;;
    -tg|--traffic-generator)
      TRAFFIC_GENERATOR_ENABLED="true"
      shift;
      ;;
    -s|--status)
      STATUS=true
      shift;shift
      ;;
    -h|--help)
      cat <<HELPMSG
Valid command line arguments:
  -ai|--auto-injection <true|false>: If you want sidecars to be auto-injected or manually injected (default: true).
  -db|--delete-bookstore <true|false>: If true, uninstall bookstore. If false, install bookstore. (default: false).
  -id|--osm-dir <dir>: Where OSM has already been cloned. Default is current directory. If not found, this script aborts.
  -c|--client-exe <name>: Cluster client executable name - valid values are "kubectl" or "oc"
  -tg|--traffic-generator: Install Kiali Traffic Generator on Bookinfo
  -h|--help : this message
HELPMSG
      exit 1
      ;;
    *)
      echo "Unknown argument [$key]. Aborting."
      exit 1
      ;;
  esac
done

if [ "${OSM_DIR}" == "" ]; then
  OSM_DIR=$PWD
  DOCS_DIR="${OSM_DIR}/docs"
fi

if [ ! -d "${OSM_DIR}" ]; then
   echo "ERROR: OSM cannot be found at: ${OSM_DIR}"
   exit 1
fi

echo "OSM is found here: ${OSM_DIR}"

CLIENT_EXE=`which ${CLIENT_EXE_NAME}`
if [ "$?" = "0" ]; then
  echo "The cluster client executable is found here: ${CLIENT_EXE}"
else
  echo "You must install the cluster client ${CLIENT_EXE_NAME} in your PATH before you can continue"
  exit 1
fi

OSM_EXE=`which osm`
if [ "$?" = "0" ]; then
  echo "The OSM executable is found here: ${OSM_EXE}"
else
  echo "You must install the OSM executable (osm cli command) in your PATH before you can continue"
  exit 1
fi

if [ "${STATUS}" == "true" ]; then
  echo "====== DEPLOYMENT AND POD STATUS FOR BOOKSTORE DEMO ====="
  for i in ${NAMESPACES}; do $CLIENT_EXE get services -n ${i}; done
  for i in ${NAMESPACES}; do $CLIENT_EXE get pods -n ${i}; done
   exit 1
fi

if [ "${DELETE_BOOKSTORE}" == "true" ]; then
  echo "====== UNINSTALLING ANY EXISTING BOOKSTORE DEMO ====="
  if [[ "$CLIENT_EXE" = *"oc" ]]; then
    for i in ${NAMESPACES}; do 
      $CLIENT_EXE adm policy remove-scc-from-group privileged system:serviceaccounts:${i}
      $CLIENT_EXE adm policy remove-scc-from-group anyuid system:serviceaccounts:${i}
      $CLIENT_EXE delete network-attachment-definition ${i}-cni -n ${i}
      $CLIENT_EXE delete project ${i}
    done    
  else
    for i in ${NAMESPACES}; do $CLIENT_EXE delete namespace ${i}; done
  fi
  echo "====== BOOKSTORE UNINSTALLED ====="
  exit 0
fi

echo "Setting up Bookstore Demo namespaces..."
# If OpenShift, we need to do some additional things
if [[ "$CLIENT_EXE" = *"oc" ]]; then
  for i in ${NAMESPACES}; do $CLIENT_EXE new-project ${i}; done
  for i in ${NAMESPACES}; do $CLIENT_EXE adm policy add-scc-to-group anyuid system:serviceaccounts:${i}; done
  for i in ${NAMESPACES}; do $CLIENT_EXE adm policy add-scc-to-group privileged system:serviceaccounts:${i}; done
else
  for i in ${NAMESPACES}; do $CLIENT_EXE create namespace ${i}; done
fi

echo "Onboarding namespaces into the mesh..."
for i in ${NAMESPACES}; do $OSM_EXE namespace add $i; done

echo "Deploying the demo..."
$CLIENT_EXE apply -f "${OSM_DIR}/docs/example/manifests/apps/"

#if [ "${AUTO_INJECTION}" == "true" ]; then
#  $CLIENT_EXE label namespace ${NAMESPACE} "istio-injection=enabled"
#  $CLIENT_EXE apply -n ${NAMESPACE} -f ${BOOKSTORE_YAML}
#else
#  $OSMCTL kube-inject -f ${BOOKSTORE_YAML} | $CLIENT_EXE apply -n ${NAMESPACE} -f -
#fi

# $CLIENT_EXE create -n ${NAMESPACE} -f ${GATEWAY_YAML}

sleep 10

echo "Bookstore Demo should be installed and starting up - here are the services:"
for i in ${NAMESPACES}; do $CLIENT_EXE get services -n ${i}; done
echo "Bookstore Demo should be installed and starting up - here are the pods:"
for i in ${NAMESPACES}; do $CLIENT_EXE get pods -n ${i}; done

# If OpenShift, we need to do some additional things
if [[ "$CLIENT_EXE" = *"oc" ]]; then
#  $CLIENT_EXE expose svc/productpage -n ${NAMESPACE}
#  $CLIENT_EXE expose svc/istio-ingressgateway --port http2 -n ${OSM_NAMESPACE}
  for i in ${NAMESPACES}; do
    cat <<NAD | $CLIENT_EXE -n ${i} create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ${i}-cni
NAD
  done
fi

