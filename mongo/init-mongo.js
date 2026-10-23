
const database = process.env.MONGO_APP_DATABASE;
const username = process.env.MONGO_APP_USERNAME;
const password = process.env.MONGO_APP_PASSWORD;

if (!database || !username || !password) {
    throw new Error("Faltan variables de entorno para crear el usuario de la aplicación");
}

const appDb = db.getSiblingDB(database);

appDb.createUser({
    user: username,
    pwd: password,
    roles: [
        { role: "readWrite", db: database }
    ]
});

print(`Usuario de aplicación creado para la base de datos: ${database}`);
