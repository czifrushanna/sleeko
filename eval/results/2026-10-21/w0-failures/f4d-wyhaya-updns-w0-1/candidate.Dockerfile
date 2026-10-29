FROM rust:1.70 AS builder
WORKDIR /app
COPY . /app
RUN cargo build --release

FROM ubuntu:22.04
EXPOSE 53/udp
WORKDIR /app
COPY --from=builder /app/target/release/updns .
ENV LOG=info,warn,error
RUN groupadd -r app && useradd -r -g app app && chown -R app:app /app
USER app
CMD ["./updns"]
