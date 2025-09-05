# Generate Python gRPC code from proto file
python -m grpc_tools.protoc -I./proto --python_out=./src/grpc_client --grpc_python_out=./src/grpc_client ./proto/contest.proto
