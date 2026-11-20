#!/bin/sh
# Entry point de los contenedores de la app.
#
# El contenedor empieza como root para poder leer las credenciales del AWS CLI
# montadas en /aws-readonly (los archivos del host solo puede leerlos su uid) y
# luego baja al usuario kenko con su-exec, como hace la imagen oficial de
# MongoDB con su clave de réplica. La copia vive en el sistema de archivos
# efímero del contenedor, no en la imagen.
#
# Si no hay montaje (por ejemplo en producción, con variables de entorno o un
# rol de IAM), se ejecuta el comando directamente.
set -eu

if [ -f /aws-readonly/credentials ]; then
    install -d -o kenko -g kenko /home/kenko/.aws
    [ -f /aws-readonly/config ] && \
        install -m 600 -o kenko -g kenko /aws-readonly/config /home/kenko/.aws/config
    install -m 600 -o kenko -g kenko /aws-readonly/credentials /home/kenko/.aws/credentials
fi

exec su-exec kenko:kenko "$@"
