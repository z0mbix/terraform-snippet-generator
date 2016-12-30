build:
	@go build

run:
	./terraform-snippet-generator -source ./terraform -provider aws -editor vim
	@# ./terraform-snippet-generator -source ./terraform -provider aws -editor atom

buildrun: build run
