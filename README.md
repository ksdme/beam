<p align="center">
    <img src="./assets/mast.png" width="100%" height="auto">
</p>

<p align="center">
    transfer pipes and files from one computer to another over ssh
</p>

<br/>

## Why SSH?

Unlike [wormhole](https://github.com/magic-wormhole/magic-wormhole) or [portal](https://github.com/SpatiumPortae/portal), Beam is built
on top of SSH. As a result, it has some advantages, including,

- 🚀 No binary installation, all you need is an ssh client
- 🔒 Built in authentication
- 📡 Support for transferring pipes, not just files

### How does it work?

On the sender end, pipe your contents into beam using something like,

```
echo hello | ssh beam.ssh.camp send
```

And, then, on the receiver end start reading the contents out using,

```
ssh beam.ssh.camp receive --progress
```

By default, Beam identifies your session using your public key. So, if you are using the same SSH keys on
both the sender and the receiver end, you do not need to use an explicit channel name. When the same key
isn't available on both the machines, you can use a random channel name. You can do this using,

```
echo hello | ssh beam.ssh.camp send --random-channel
```

For example, here's a demo tail-ing a log file from one machine on the other,

<img src="./assets/logs-beam-demo.gif" width="100%" height="auto" style="display: block; min-height: 80px;">

### Caveats

- SSH connections cannot be load balanced or geo-routed. So, unless you explicitly use the host closest to you,
you might notice low transfer rates.

- Beam cannot support end-to-end encrypted buffers. While data is encrypted during transfer to and from the Beam
host, it’s decrypted temporarily before being re-encrypted and forwarded. The host only holds a small buffer
(typically 1 kB) of unencrypted data at any time and never stores the full stream. For extra security, you
can encrypt your files or pipes before sending them through Beam.

### Self Hosting

Hosting a beam server is a simple and lightweight affair. It doesn't depend on any services. You can build a
binary of the server yourself, or, use the [docker image](https://hub.docker.com/repository/docker/ksdme/beam/general).
