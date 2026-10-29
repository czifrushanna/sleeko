FROM rust:1.70 AS builder
WORKDIR /root
COPY . /root
RUN cargo build --release

FROM ubuntu:22.04
EXPOSE 53/udp
WORKDIR /root
COPY --from=builder /root/target/release/updns .
ENV LOG=info,warn,error
RUN groupadd -r app && useradd -r -g app app && chown -R app:app /root
USER app
CMD ["./updns"]
