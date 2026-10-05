import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import {
  applicationVariableCreateBody,
  applicationVariableFieldErrors,
  applicationVariablePath,
  applicationVariableUpdateBody,
  createApplicationVariable,
  deleteApplicationVariable,
  updateApplicationVariable,
  variableErrorMessage,
  type VariableMutationClient,
} from './application-variables'

const APP_ID = '1d13e36a-a68a-45b1-8380-5556a65e7f25'
const ORG_ID = '9c1b7e2a-4f31-4c0a-9a55-2b6d8e0f11aa'
const VARIABLE_ID = '742c2b91-80b8-4a8e-93ba-82ee7480d285'

describe('application variables', () => {
  it('builds an application-scoped create payload without project or environment ids', () => {
    const body = applicationVariableCreateBody({
      organizationId: ORG_ID,
      applicationId: APP_ID,
      key: ' PORT ',
      value: '8000',
    })
    assert.deepEqual(body, {
      organizationId: ORG_ID,
      scope: 'APPLICATION',
      applicationId: APP_ID,
      key: 'PORT',
      value: '8000',
    })
    assert.equal('projectId' in body, false)
    assert.equal('environmentId' in body, false)
  })

  it('patches key and value on the variable path', async () => {
    const calls: Array<{ method: string; path: string; body?: unknown }> = []
    const client: VariableMutationClient = {
      async post() {
        throw new Error('create is separate')
      },
      async patch<T>(path: string, body?: unknown): Promise<T> {
        calls.push({ method: 'patch', path, body })
        return { variable: { id: VARIABLE_ID } } as T
      },
      async delete() {
        throw new Error('delete is separate')
      },
    }
    await updateApplicationVariable(client, VARIABLE_ID, { key: ' DATABASE_URL ', value: 'postgres://db' })
    assert.deepEqual(calls, [
      {
        method: 'patch',
        path: applicationVariablePath(VARIABLE_ID),
        body: applicationVariableUpdateBody({ key: ' DATABASE_URL ', value: 'postgres://db' }),
      },
    ])
    assert.deepEqual(calls[0].body, { key: 'DATABASE_URL', value: 'postgres://db' })
  })

  it('deletes by variable id', async () => {
    const calls: string[] = []
    const client: VariableMutationClient = {
      async post() {
        throw new Error('create is separate')
      },
      async patch() {
        throw new Error('update is separate')
      },
      async delete<T>(path: string): Promise<T> {
        calls.push(path)
        return { ok: true } as T
      },
    }
    await deleteApplicationVariable(client, VARIABLE_ID)
    assert.deepEqual(calls, [`/variables/${VARIABLE_ID}`])
  })

  it('posts the application create body', async () => {
    const posts: Array<{ path: string; body: unknown }> = []
    const client: VariableMutationClient = {
      async post<T>(path: string, body?: unknown): Promise<T> {
        posts.push({ path, body })
        return { variable: { id: VARIABLE_ID } } as T
      },
      async patch() {
        throw new Error('update is separate')
      },
      async delete() {
        throw new Error('delete is separate')
      },
    }
    await createApplicationVariable(client, {
      organizationId: ORG_ID,
      applicationId: APP_ID,
      key: 'APP_ENV',
      value: 'production',
    })
    assert.deepEqual(posts, [
      {
        path: '/variables',
        body: {
          organizationId: ORG_ID,
          scope: 'APPLICATION',
          applicationId: APP_ID,
          key: 'APP_ENV',
          value: 'production',
        },
      },
    ])
  })

  it('rejects keys and values the control plane would reject', () => {
    assert.deepEqual(applicationVariableFieldErrors('1PORT', 'ok'), {
      key: 'Key must match [A-Za-z_][A-Za-z0-9_]*.',
    })
    assert.deepEqual(applicationVariableFieldErrors(`_${'A'.repeat(256)}`, ''), {
      key: 'Key must be at most 256 characters.',
    })
    assert.equal(applicationVariableFieldErrors('PORT', 'x'.repeat(8193)).value, 'Value must be at most 8192 characters.')
    assert.deepEqual(applicationVariableFieldErrors('PORT', ''), {})
    assert.deepEqual(applicationVariableFieldErrors('_OK', 'x'.repeat(8192)), {})
  })

  it('keeps variable values out of error text', () => {
    const value = 'postgres://user:secret@db/app'
    const err = new Error(`invalid variable value ${value}`)
    assert.equal(variableErrorMessage(err, [value]), 'The variable could not be saved.')
    assert.equal(variableErrorMessage(new Error('variable key already exists in scope'), [value]), 'variable key already exists in scope')
    assert.equal(JSON.stringify(variableErrorMessage(err, [value])).includes(value), false)
  })
})
