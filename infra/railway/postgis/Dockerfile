# syntax=docker/dockerfile:1

FROM postgis/postgis:16-3.5-alpine

COPY infra/railway/postgis/entrypoint.sh /usr/local/bin/wadd-postgis-entrypoint
RUN chmod +x /usr/local/bin/wadd-postgis-entrypoint

ENTRYPOINT ["wadd-postgis-entrypoint"]
CMD ["postgres"]
