# Use the official minimal Alpine-based Go image
FROM golang:1.24-alpine

# Install common dev tools (git and build-base for cgo support if needed)
RUN apk add --no-cache git build-base bash

# Configure the non-root user
ARG USERNAME=gouser
ARG USER_UID=1000
ARG USER_GID=1000

# Add the user and set up their home directory
RUN addgroup -g $USER_GID $USERNAME \
    && adduser -u $USER_UID -G $USERNAME -D -s /bin/bash $USERNAME \
    && mkdir -p /workspace \
    && chown -R $USERNAME:$USERNAME /workspace

# Point HOME and GOCACHE to the new user's space
USER $USERNAME
ENV HOME=/home/$USERNAME
ENV GOCACHE=$HOME/.cache/go-build
ENV PATH=/usr/local/go/bin/:$PATH
WORKDIR /workspace

# Keep the container running so you can 'exec' into it
CMD ["sleep", "infinity"]