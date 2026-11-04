"""Phase 3 management forms exercised in the real browser."""
from urllib.parse import urlencode
from playwright.sync_api import expect


def run_management_checks(page, origin, bearer, passed):
    headers = {'Authorization': bearer}

    def api(method, path, data=None, statuses=(200,)):
        response = page.request.fetch(origin + path, method=method, headers=headers, data=data)
        assert response.status in statuses, (method, path, response.status, response.text())
        return response.json().get('data')

    def row(email):
        return page.get_by_role('row').filter(has=page.get_by_role('cell', name=email, exact=True))

    def confirm():
        page.once('dialog', lambda dialog: dialog.accept())

    page.locator('nav a[href="/admin-page/users"]').click()
    expect(page.get_by_role('heading', name='Users', exact=True)).to_be_visible()
    page.get_by_role('button', name='New user', exact=True).click()
    page.get_by_label('Email', exact=True).fill('browser-user@example.invalid')
    page.get_by_label('Display name', exact=True).fill('Browser user')
    page.get_by_label('Password', exact=False).fill('Browser-original-123!')
    page.get_by_role('button', name='Create user', exact=True).click()
    expect(row('browser-user@example.invalid')).to_be_visible()
    row('browser-user@example.invalid').get_by_role('button', name='Edit', exact=True).click()
    page.get_by_label('Email', exact=True).fill('browser-edited@example.invalid')
    page.get_by_label('Display name', exact=True).fill('Browser edited')
    confirm()
    page.get_by_role('button', name='Save user', exact=True).click()
    expect(row('browser-edited@example.invalid')).to_be_visible()
    expect(row('browser-edited@example.invalid').get_by_role('cell', name='Browser edited', exact=True)).to_be_visible()
    passed('P3 user create and edit update the real table')

    confirm()
    row('browser-edited@example.invalid').get_by_role('button', name='Disable', exact=True).click()
    expect(row('browser-edited@example.invalid').get_by_text('Disabled', exact=True)).to_be_visible()
    page.get_by_label('Status', exact=True).select_option('disabled')
    expect(row('browser-edited@example.invalid')).to_be_visible()
    confirm()
    row('browser-edited@example.invalid').get_by_role('button', name='Enable', exact=True).click()
    expect(row('browser-edited@example.invalid')).to_have_count(0)
    page.get_by_label('Status', exact=True).select_option('all')
    expect(row('browser-edited@example.invalid').get_by_text('Active', exact=True)).to_be_visible()
    passed('P3 account disable/enable and status filter work')

    row('browser-edited@example.invalid').get_by_role('button', name='Reset password', exact=True).click()
    page.get_by_label('Password', exact=False).fill('Browser-replacement-456!')
    confirm()
    page.locator('form').filter(has=page.get_by_label('Password', exact=False)).get_by_role('button', name='Reset password', exact=True).click()
    expect(page.get_by_text('Password reset. Existing grants and sessions have been invalidated.', exact=True)).to_be_visible()
    confirm()
    row('browser-edited@example.invalid').get_by_role('button', name='Delete', exact=True).click()
    expect(row('browser-edited@example.invalid')).to_have_count(0)
    confirm()
    row('regression@example.invalid').get_by_role('button', name='Disable', exact=True).click()
    expect(page.get_by_text('cannot remove or disable the last active administrator', exact=True)).to_be_visible()
    expect(row('regression@example.invalid').get_by_text('Active', exact=True)).to_be_visible()
    passed('P3 password reset, deletion and last-admin error feedback work')

    ids = []
    for i in range(21):
        result = api('POST', '/api/admin/users', {'email': f'browser-page-{i:02d}@example.invalid', 'display_name': 'Browser pagination', 'password': 'Browser-pagination-123!'})
        ids.append(result['id'])
    page.get_by_label('Page size', exact=True).select_option('10')
    page.get_by_role('searchbox').fill('browser-page-')
    page.get_by_role('button', name='Search', exact=True).click()
    expect(page.get_by_text('21 results · Page 1 / 3', exact=True)).to_be_visible()
    expect(page.locator('tbody tr')).to_have_count(10)
    page.get_by_role('button', name='Next', exact=True).click()
    expect(page.get_by_text('21 results · Page 2 / 3', exact=True)).to_be_visible()
    page.get_by_role('button', name='Next', exact=True).click()
    expect(page.get_by_text('21 results · Page 3 / 3', exact=True)).to_be_visible()
    expect(page.locator('tbody tr')).to_have_count(1)
    confirm()
    page.locator('tbody tr').get_by_role('button', name='Delete', exact=True).click()
    expect(page.get_by_text('20 results · Page 2 / 2', exact=True)).to_be_visible()
    for id_ in ids:
        api('DELETE', f'/api/admin/users/{id_}', statuses=(200, 404))
    page.get_by_role('searchbox').fill('')
    page.get_by_role('button', name='Search', exact=True).click()
    expect(row('regression@example.invalid')).to_be_visible()
    passed('P3 search, pagination and clamping after last-row deletion work')

    page.locator('nav a[href="/admin-page/clients"]').click()
    nullable = row('Nullable policy client')
    expect(nullable).to_be_visible()
    result = api('GET', '/api/admin/clients?q=Nullable')
    for field in ('scopes', 'redirect_uris', 'grant_types'):
        assert result['items'][0][field] == []
    nullable.get_by_role('button', name='Edit', exact=True).click()
    expect(page.get_by_label('Allowed scopes', exact=True)).to_have_value('')
    expect(page.get_by_label('Redirect URIs (one per line)', exact=True)).to_have_value('https://nullable.example.invalid/callback')
    page.get_by_role('button', name='Cancel', exact=True).click()
    passed('SQL NULL scopes and empty client arrays render and open for editing')

    # An older running backend can still return null while Vite loads the fix.
    def legacy_null_arrays(route):
        response = route.fetch()
        payload = response.json()
        for client in payload['data']['items']:
            if client['display_name'] == 'Nullable policy client':
                for field in ('scopes', 'redirect_uris', 'grant_types'):
                    client[field] = None
        route.fulfill(response=response, json=payload)

    page.route(origin + '/api/admin/clients?**', legacy_null_arrays)
    page.locator('nav a[href="/admin-page/users"]').click()
    page.locator('nav a[href="/admin-page/clients"]').click()
    expect(nullable).to_be_visible()
    nullable.get_by_role('button', name='Edit', exact=True).click()
    expect(page.get_by_label('Allowed scopes', exact=True)).to_have_value('')
    expect(page.get_by_label('Redirect URIs (one per line)', exact=True)).to_have_value('https://nullable.example.invalid/callback')
    expect(page.get_by_role('checkbox', name='authorization_code', exact=True)).not_to_be_checked()
    page.get_by_role('button', name='Cancel', exact=True).click()
    viewport = page.viewport_size
    page.set_viewport_size({'width': 390, 'height': 844})
    card = page.locator('article').filter(has=page.get_by_role('heading', name='Nullable policy client', exact=True))
    expect(card).to_be_visible()
    card.get_by_role('button', name='Edit', exact=True).click()
    expect(page.get_by_label('Allowed scopes', exact=True)).to_have_value('')
    page.get_by_role('button', name='Cancel', exact=True).click()
    page.set_viewport_size(viewport)
    page.unroute(origin + '/api/admin/clients?**', legacy_null_arrays)
    passed('legacy null client arrays render and edit on desktop and mobile')

    page.get_by_role('button', name='New client', exact=True).click()
    page.get_by_label('Display name', exact=True).fill('Browser client')
    page.get_by_label('Redirect URIs (one per line)', exact=True).fill('https://browser.example.invalid/callback')
    page.get_by_label('Allowed scopes', exact=True).fill('profile')
    page.get_by_role('button', name='Create client', exact=True).click()
    expect(row('Browser client')).to_be_visible()
    expect(page.locator('code')).to_be_visible()
    old_secret = page.locator('code').inner_text()
    assert len(old_secret) >= 32
    row('Browser client').get_by_role('button', name='Edit', exact=True).click()
    page.get_by_label('Display name', exact=True).fill('Browser edited client')
    page.get_by_label('Redirect URIs (one per line)', exact=True).fill('https://browser.example.invalid/new-callback\nhttps://browser.example.invalid/second')
    page.get_by_label('Allowed scopes', exact=True).fill('profile avatar')
    confirm()
    page.get_by_role('button', name='Save client', exact=True).click()
    expect(row('Browser edited client')).to_be_visible()
    expect(row('Browser edited client').get_by_text('https://browser.example.invalid/second', exact=True)).to_be_visible()
    confirm()
    row('Browser edited client').get_by_role('button', name='Disable', exact=True).click()
    expect(row('Browser edited client').get_by_role('button', name='Enable', exact=True)).to_be_visible()
    confirm()
    row('Browser edited client').get_by_role('button', name='Enable', exact=True).click()
    expect(row('Browser edited client').get_by_role('button', name='Disable', exact=True)).to_be_visible()
    passed('P3 client create/edit with multiple callbacks and disable/enable work')

    confirm()
    row('Browser edited client').get_by_role('button', name='Reset secret', exact=True).click()
    expect(page.locator('code')).to_be_visible()
    assert page.locator('code').inner_text() != old_secret
    result = api('GET', '/api/admin/clients?' + urlencode({'q': 'Browser edited client'}))
    assert 'secret' not in result['items'][0]
    protected = row('Regression admin')
    expect(protected.get_by_text('Protected management client', exact=True)).to_be_visible()
    expect(protected.get_by_role('button')).to_have_count(0)
    confirm()
    row('Browser edited client').get_by_role('button', name='Delete', exact=True).click()
    expect(row('Browser edited client')).to_have_count(0)
    passed('P3 secret rotation, secret redaction and protected management client work')
