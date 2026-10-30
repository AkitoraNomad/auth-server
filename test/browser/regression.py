"""Browser regression against the isolated Go integration server.

Real PostgreSQL/Redis and application routes; Turnstile is stubbed by the test
server and widget route. No production configuration or accounts are used.
"""
import os
import re
import shutil
from playwright.sync_api import sync_playwright, expect
from management import run_management_checks

origin = os.environ['AUTH_TEST_ORIGIN']
checks = []

def passed(name):
    checks.append(name)
    print('PASS:', name, flush=True)

with sync_playwright() as p:
    browser = p.chromium.launch(executable_path=os.environ.get('AUTH_TEST_CHROMIUM') or shutil.which('chromium'), headless=True, args=['--no-sandbox'])
    page = browser.new_page()
    page.set_default_timeout(10000)
    errors = []
    page.on('request', lambda req: print('Consent request origin:', req.headers.get('origin'), 'fetch-site:', req.headers.get('sec-fetch-site'), flush=True) if req.method == 'POST' and req.url.endswith('/oauth2/authorize') else None)
    page.on('pageerror', lambda err: errors.append(str(err)))
    page.route('https://challenges.cloudflare.com/**', lambda route: route.fulfill(status=200, content_type='application/javascript', body=''))
    page.route('https://telegram.org/**', lambda route: route.abort())
    page.route('https://cdn.bootcss.com/**', lambda route: route.abort())

    def sign_in(scopes=('manage:system', 'manage:clients')):
        page.goto(origin + '/admin-page/users')
        page.wait_for_url(re.compile(r'/(login|auth)\?'))
        if '/login' in page.url and not '/admin-page/login' in page.url:
            page.locator('#email').fill(os.environ['AUTH_TEST_EMAIL'])
            page.locator('#password').fill(os.environ['AUTH_TEST_PASSWORD'])
            page.evaluate("turnstileCallback('regression-fixture')")
            page.locator('#loginBtn').click()
        page.locator('#consentForm').wait_for()
        for checkbox in page.locator('input[name=consented]').all():
            checkbox.set_checked(checkbox.get_attribute('value') in scopes)
        page.get_by_role('button', name='Allow access').click()
        page.wait_for_url('**/admin-page/users')

    sign_in()
    expect(page.get_by_role('heading', name='Users', exact=True)).to_be_visible()
    expect(page.get_by_role('cell', name=os.environ['AUTH_TEST_EMAIL'], exact=True)).to_be_visible()
    passed('real password login, S256 callback, deep-link return and user list')
    assert page.evaluate('Object.keys(localStorage).length') == 0
    assert page.evaluate("sessionStorage.getItem('auth_admin_pkce_v1')") is None
    passed('tokens stay out of localStorage and pending PKCE is cleared')

    for path, heading in [('clients', 'OAuth2 clients'), ('admins', 'Administrators'), ('sessions', 'Sessions')]:
        page.locator(f'nav a[href="/admin-page/{path}"]').click()
        expect(page.get_by_role('heading', name=heading, exact=True)).to_be_visible()
        expect(page.locator('.loading')).to_have_count(0)
        passed(path + ' list loads through shared hook')
    page.locator('nav a[href="/admin-page/clients"]').click()
    page.get_by_role('button', name='New client', exact=True).click()
    expect(page.get_by_text('Implicit', exact=True)).to_have_count(0)
    passed('disabled implicit grant is absent from client form')

    user_requests = []
    def expire_first_request(route):
        user_requests.append(route.request.headers.get('authorization'))
        if len(user_requests) == 1:
            route.fulfill(status=401, content_type='application/json', body='{"code":1,"msg":"expired"}')
        else:
            route.continue_()
    page.route(origin + '/api/admin/users?**', expire_first_request)
    page.locator('nav a[href="/admin-page/users"]').click()
    expect(page.get_by_role('cell', name=os.environ['AUTH_TEST_EMAIL'], exact=True)).to_be_visible()
    assert len(user_requests) == 2 and user_requests[0] != user_requests[1]
    passed('401 refresh rotates the token and retries API with current auth state')
    page.unroute(origin + '/api/admin/users?**')
    page.locator('nav a[href="/admin-page/clients"]').click()
    expect(page.get_by_role('heading', name='OAuth2 clients', exact=True)).to_be_visible()

    run_management_checks(page, origin, user_requests[-1], passed)

    pending = []
    page.route(origin + '/logout', lambda route: pending.append(route))
    page.get_by_role('button', name='Sign out', exact=True).click()
    expect(page.get_by_role('button', name='Signing out...')).to_be_disabled()
    assert '/admin-page/clients' in page.url
    assert len(pending) == 1
    pending.pop().fulfill(status=503, body='unavailable')
    expect(page.get_by_text('Server sign out failed. Please try again.', exact=True)).to_be_visible()
    expect(page.get_by_role('button', name='Sign out', exact=True)).to_be_enabled()
    assert '/admin-page/clients' in page.url
    passed('logout waits for response, retains session and exposes failure')
    page.unroute(origin + '/logout')
    page.get_by_role('button', name='Sign out', exact=True).click()
    page.wait_for_url('**/admin-page/login')
    expect(page.get_by_role('button', name='Sign in with Auth Server')).to_be_visible()
    passed('retry performs real server logout before navigation')

    sign_in(('manage:clients',))
    expect(page.get_by_role('heading', name='Permission required', exact=True)).to_be_visible()
    expect(page.locator('nav a[href="/admin-page/users"]')).to_have_count(0)
    passed('missing scope shows 403 and hides unauthorized navigation')
    page.get_by_role('button', name='Sign out', exact=True).first.click()
    page.wait_for_url('**/admin-page/login')

    # A reload preserves only SSO; consent is deliberately per authorization.
    sign_in()
    page.reload()
    page.locator('#consentForm').wait_for()
    for checkbox in page.locator('input[name=consented]').all():
        checkbox.check()
    page.get_by_role('button', name='Allow access').click()
    page.wait_for_url('**/admin-page/users')
    expect(page.get_by_role('heading', name='Users', exact=True)).to_be_visible()
    passed('deep-link reload reauthorizes through SSO and preserves return path')
    page.set_viewport_size({'width': 390, 'height': 844})
    expect(page.get_by_role('heading', name='Users', exact=True)).to_be_visible()
    assert page.evaluate('document.documentElement.scrollWidth <= window.innerWidth')
    page.get_by_label('Open navigation').click()
    expect(page.locator('nav a[href="/admin-page/clients"]')).to_be_visible()
    page.locator('nav a[href="/admin-page/clients"]').click()
    expect(page.get_by_role('heading', name='OAuth2 clients', exact=True)).to_be_visible()
    page.keyboard.press('Tab')
    assert page.evaluate("document.activeElement !== document.body")
    passed('narrow layout, drawer navigation and keyboard focus')

    response = page.request.get(origin + '/admin-page/assets/does-not-exist.js')
    assert response.status == 404
    page.goto(origin + '/admin-page/unknown-regression-page')
    expect(page.get_by_role('heading', name='Page not found', exact=True)).to_be_visible()
    passed('missing assets and unknown routes return expected 404 behavior')
    page.goto(origin + '/admin-page/callback?code=bad&state=bad')
    page.wait_for_url('**/admin-page/login')
    expect(page.get_by_text('This sign-in request is no longer valid. Please start again.', exact=True)).to_be_visible()
    passed('unsolicited callback is rejected')
    page.goto(origin + '/admin-page/users')
    page.locator('#consentForm').wait_for()
    page.get_by_role('button', name='Deny', exact=True).click()
    page.wait_for_url('**/admin-page/login')
    expect(page.get_by_text('The user denied the authorization request', exact=True)).to_be_visible()
    assert page.evaluate("sessionStorage.getItem('auth_admin_pkce_v1')") is None
    passed('denied consent returns an error and clears pending PKCE')
    assert not errors, errors
    print(f'Browser regression: {len(checks)} checks passed, no uncaught page errors.', flush=True)
    browser.close()
