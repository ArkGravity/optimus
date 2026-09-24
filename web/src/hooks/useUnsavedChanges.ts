import { inject, onScopeDispose } from 'vue'
import { routerKey } from 'vue-router'
import { useI18n } from '@/hooks/useI18n'
import { useAuthStore } from '@/stores/auth'

export function useUnsavedChanges(isDirty: () => boolean) {
  const router = inject(routerKey, undefined)
  const auth = useAuthStore()
  const { t } = useI18n()
  const removeGuard = router?.beforeEach((to, from) => {
    if (to.path === from.path || !auth.accessToken || !isDirty()) return true
    return window.confirm(t('workspace.unsaved'))
  })
  function beforeUnload(event: BeforeUnloadEvent) {
    if (!auth.accessToken || !isDirty()) return
    event.preventDefault()
    event.returnValue = ''
  }
  window.addEventListener('beforeunload', beforeUnload)
  onScopeDispose(() => {
    removeGuard?.()
    window.removeEventListener('beforeunload', beforeUnload)
  })
}
