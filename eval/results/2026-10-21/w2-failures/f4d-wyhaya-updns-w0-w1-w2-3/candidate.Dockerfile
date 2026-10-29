FROM rust:1.27.2 as builder
WORKDIR /root
COPY . /root
RUN cargo build --release

FROM ubuntu:18.04
EXPOSE 53/udp
WORKDIR /root
COPY --from=builder /root/target/release/updns /root/updns
ENV LOG=info,warn,error
RUN useradd -r -u 1000 updns && chown updns:updns /root
USER updns
CMD ["./updns"]
