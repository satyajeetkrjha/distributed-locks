# distributed-locks

#on one terminal start client and hit
<br></br>
go run main.go --name=lock1  
go run main.go --name=lock2  
go run main.go --name=lock3  

<br></br>
#on other terminal start server and hit
go run main.go -server
