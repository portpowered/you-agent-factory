import test from 'node:test'
import assert from 'node:assert/strict'
import { PiAcpAgent } from '../src/acp/agent.js'
import { PiAcpSession } from '../src/acp/session.js'
import { FakeAgentSideConnection, FakePiRpcProcess, asAgentConn } from './helpers/fakes.js'

async function promptWithEvents(events: Record<string, unknown>[]) {
  const conn = new FakeAgentSideConnection()
  const proc = new FakePiRpcProcess()
  const session = new PiAcpSession({
    sessionId: 's1', cwd: process.cwd(), mcpServers: [],
    proc: proc as any, conn: asAgentConn(conn), fileCommands: []
  })
  const agent = new PiAcpAgent(asAgentConn(conn))
  ;(agent as any).sessions = { maybeGet: () => session, get: () => session }
  const prompt = agent.prompt({ sessionId: 's1', prompt: [{ type: 'text', text: 'hello' }] } as any)
  // Let the async session lookup start the prompt before delivering Pi events.
  for (let i = 0; i < 10 && proc.prompts.length === 0; i++) await Promise.resolve()
  assert.equal(proc.prompts.length, 1)
  for (const event of events) proc.emit(event)
  proc.emit({ type: 'agent_settled' })
  return prompt
}

test('final typed Pi error becomes an ACP prompt failure', async () => {
  await assert.rejects(
    promptWithEvents([
      { type: 'message_end', message: { role: 'assistant', stopReason: 'error' } },
      { type: 'agent_end' }
    ]),
    error => {
      assert.equal((error as any).code, -32603)
      assert.match(String((error as Error).message), /Pi assistant turn failed/)
      return true
    }
  )
})

test('failed attempt followed by successful retry completes the ACP prompt', async () => {
  const response = await promptWithEvents([
    { type: 'message_end', message: { role: 'assistant', stopReason: 'error' } },
    { type: 'agent_end', willRetry: true },
    { type: 'agent_start' },
    { type: 'message_end', message: { role: 'assistant', stopReason: 'stop' } },
    { type: 'agent_end', willRetry: false }
  ])
  assert.equal(response.stopReason, 'end_turn')
})

test('known Pi connection error adds a narrow failureKind marker', async () => {
  await assert.rejects(
    promptWithEvents([
      {
        type: 'message_end',
        message: { role: 'assistant', stopReason: 'error', errorMessage: 'Connection error.' }
      },
      { type: 'agent_end' }
    ]),
    error => {
      assert.equal((error as any).code, -32603)
      assert.match(String((error as Error).message), /Pi assistant turn failed/)
      assert.deepEqual((error as any).data, {
        provider: 'pi',
        outcome: 'error',
        failureKind: 'model_connection'
      })
      return true
    }
  )
})

test('other Pi errors stay generic without leaking the Pi error message', async () => {
  await assert.rejects(
    promptWithEvents([
      {
        type: 'message_end',
        message: { role: 'assistant', stopReason: 'error', errorMessage: 'arbitrary provider detail' }
      },
      { type: 'agent_end' }
    ]),
    error => {
      assert.equal((error as any).code, -32603)
      assert.deepEqual((error as any).data, { provider: 'pi', outcome: 'error' })
      assert.ok(!String((error as Error).message).includes('arbitrary provider detail'))
      return true
    }
  )
})

test('connection error followed by successful retry completes the ACP prompt', async () => {
  const response = await promptWithEvents([
    {
      type: 'message_end',
      message: { role: 'assistant', stopReason: 'error', errorMessage: 'Connection error.' }
    },
    { type: 'agent_end', willRetry: true },
    { type: 'agent_start' },
    { type: 'message_end', message: { role: 'assistant', stopReason: 'stop' } },
    { type: 'agent_end', willRetry: false }
  ])
  assert.equal(response.stopReason, 'end_turn')
})

test('connection error followed by a different final error is not misclassified as model_connection', async () => {
  await assert.rejects(
    promptWithEvents([
      {
        type: 'message_end',
        message: { role: 'assistant', stopReason: 'error', errorMessage: 'Connection error.' }
      },
      { type: 'agent_end', willRetry: true },
      { type: 'agent_start' },
      {
        type: 'message_end',
        message: { role: 'assistant', stopReason: 'error', errorMessage: 'rate limited' }
      },
      { type: 'agent_end', willRetry: false }
    ]),
    error => {
      assert.equal((error as any).code, -32603)
      assert.deepEqual((error as any).data, { provider: 'pi', outcome: 'error' })
      return true
    }
  )
})

test('connection error followed by a final error without errorMessage is not misclassified as model_connection', async () => {
  await assert.rejects(
    promptWithEvents([
      {
        type: 'message_end',
        message: { role: 'assistant', stopReason: 'error', errorMessage: 'Connection error.' }
      },
      { type: 'agent_end', willRetry: true },
      { type: 'agent_start' },
      { type: 'message_end', message: { role: 'assistant', stopReason: 'error' } },
      { type: 'agent_end', willRetry: false }
    ]),
    error => {
      assert.equal((error as any).code, -32603)
      assert.deepEqual((error as any).data, { provider: 'pi', outcome: 'error' })
      return true
    }
  )
})
