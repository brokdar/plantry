import { rm } from "node:fs/promises"

const DB_PATH = "/tmp/plantry-e2e.db"

export default async function globalSetup() {
  await rm(DB_PATH, { force: true })
  await rm(DB_PATH + "-wal", { force: true })
  await rm(DB_PATH + "-shm", { force: true })
}
