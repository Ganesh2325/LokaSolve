# The stock PostGIS 16 image is used directly. Its bundled apt repository no longer
# publishes the Debian release needed to install postgresql-16-pgvector. pgvector is
# not required until semantic retrieval, and it will be added with a base image that
# can install it.
FROM postgis/postgis:16-3.5
