import axios from 'axios'
import MockAdapter from 'axios-mock-adapter'
import { expect, it } from 'vitest'
import { createK8sStorageApi } from './storage'

it('unwraps the backend envelope and scopes PVC namespace requests', async () => {
  const http = axios.create()
  const mock = new MockAdapter(http)
  const data = { items: [{ name: 'claim', namespace: 'prod', access_modes: [] }], truncated: false }
  mock.onGet('/k8s/clusters/7/storage/persistentvolumeclaims', { params: { namespace: 'prod' } }).reply(200, { code: 0, data, message: '' })
  expect(await createK8sStorageApi(http).list(7, 'persistentvolumeclaims', { namespace: 'prod' })).toEqual(data)
  mock.restore()
})
