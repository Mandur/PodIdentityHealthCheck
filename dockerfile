FROM golang:1.14 as builder

WORKDIR /build
COPY cmd/ .
RUN go get -d -v
RUN ls
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s" -o /build/main 
RUN ls /build
RUN chmod +x /build/main

FROM scratch as runner

COPY --from=builder /build/main .
CMD ["./main"]