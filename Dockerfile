FROM scratch
ENTRYPOINT ["/mdcmux"]
CMD ["start", "--json", "-c", "/etc/mdcmux.json"]
VOLUME /etc
COPY mdcmux /
