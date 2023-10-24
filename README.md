# distributed-locks

on one terminal start client and hit
go run main.go --name=lock1  
go run main.go --name=lock2  
go run main.go --name=lock3  

on other terminal start server and hit
go run main.go -server
