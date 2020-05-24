
NAME=${1?Error: no name given}
echo $NAME

faas-cli deploy --image csce-6snvrz2.cs.tamu.edu:5000/$NAME --name $NAME --constraint node.role==worker
sleep 25s
faas-cli rm $NAME