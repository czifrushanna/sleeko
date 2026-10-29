FROM rust:1.27.2-slim-buster AS builder
WORKDIR /root
COPY . /root
RUN cargo build --release

FROM ubuntu:20.04
EXPOSE 53/udp
WORKDIR /app
RUN groupadd --system updns && useradd --system --gid updns updns
COPY --from=builder /root/target/release/updns /app/updns
ENV LOG=info,warn,error
USER updns
CMD ["./updns"]
