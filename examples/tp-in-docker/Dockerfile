# Demo Dockerfile for running telepresence from within a docker container.
#
#   docker run \
#      --cap-add=NET_ADMIN
#      --device /dev/net/tun:/dev/net/tun \
#      -v ~/.kube/config:/root/.kube/config \
#      -it --rm tp-in-docker /bin/bash
#
FROM ubuntu

# Install Telepresence prerequisites
RUN apt-get update && \
    apt-get install -y curl sudo iproute2 iptables openssh-client sshfs

# Download and install kubectl
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Get the telepresence binary from the download site
#  RUN sudo curl -fL https://app.getambassador.io/download/tel2/linux/amd64/latest/telepresence -o telepresence
# or copy your own build (you probably want to combine this with --env TELEPRESENCE_REGISTRY=<your docker repo>)
COPY telepresence telepresence

# Install the telepresence binary
RUN sudo install -o root -g root -m 0755 telepresence /usr/local/bin/telepresence

# The docker registry used when Telepresence installs the traffic-manager and traffic-agent
ENV TELEPRESENCE_REGISTRY=docker.io/datawire

# Optional: install python3 to make it easy to intercept and run a webservice, e.g.
#
#   telepresence intercept <some service> --port 9000 python3 -m http.servver 9000
#
RUN apt-get install -y python3

# Add some conventient aliases to .bashrc
RUN echo '\
alias t=telepresence\n\
alias k=kubectl\n\
alias clog="tail -n100 -F ~/.cache/telepresence/logs/connector.log"\n\
alias dlog="tail -n100 -F ~/.cache/telepresence/logs/daemon.log"\n\
' >> /root/.bashrc

