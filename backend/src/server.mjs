import { createServer } from 'node:http'

const port = Number(process.env.PORT || 3000)

createServer((request, response) => {
  if (request.url === '/health') {
    response.writeHead(200, { 'content-type': 'application/json' })
    response.end(JSON.stringify({ status: 'ok' }))
    return
  }

  response.writeHead(404)
  response.end()
}).listen(port, () => {
  console.log(`Backend running on http://localhost:${port}`)
})
