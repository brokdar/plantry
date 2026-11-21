import { rm } from "node:fs/promises"

const DB_PATHS = [
  process.env.E2E_DB_PATH ?? "/tmp/plantry-e2e.db",
  process.env.E2E_DB_PATH_GLOBAL ?? "/tmp/plantry-e2e-global.db",
]

export default async function globalSetup() {
  for (const db of DB_PATHS) {
    await rm(db, { force: true })
    await rm(db + "-wal", { force: true })
    await rm(db + "-shm", { force: true })
  }
}
