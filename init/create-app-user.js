const appDb = process.env.MONGO_APP_DB;

db = db.getSiblingDB(appDb);
db.createUser({
    user: process.env.MONGO_APP_USER,
    pwd: process.env.MONGO_APP_PASSWORD,
    roles: [{ role: "readWrite", db: appDb }],
});