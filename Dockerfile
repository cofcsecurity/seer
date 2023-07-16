FROM golang:1.20.6-bookworm as builder

COPY . /root/seer/.
WORKDIR /root/seer

RUN go build -o seer

FROM debian:bookworm

COPY --from=builder /root/seer/seer /usr/local/bin/seer

# Set up bash completion
RUN apt update && apt install bash-completion -y
RUN mkdir /etc/bash_completion.d/
RUN seer completion bash > /etc/bash_completion.d/seer
RUN echo "source /etc/bash_completion" >> /etc/bash.bashrc

# Optional utils
RUN apt install screen netcat-traditional iputils-ping -y

# Set up test data
RUN useradd alice && usermod -aG sudo alice
RUN useradd bob

ENTRYPOINT [ "/bin/bash" ]