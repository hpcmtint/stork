import { HttpErrorResponse, HttpInterceptor, HttpEvent, HttpHandler, HttpRequest } from '@angular/common/http'
import { Router } from '@angular/router'
import { Injectable } from '@angular/core'
import { throwError, of, Observable } from 'rxjs'
import { catchError } from 'rxjs/operators'

import { AuthService } from './auth.service'
import { ServerDataService } from './server-data.service'

@Injectable()
export class AuthInterceptor implements HttpInterceptor {
    constructor(private router: Router, private auth: AuthService, private serverData: ServerDataService) {}

    private handleAuthError(err: HttpErrorResponse): Observable<any> {
        // The server sometimes returns HTTP Error 403 when the session expires
        // but the browser still remembers it in the local storage. The 403 may
        // also be returned for the authorized user but not having access to the
        // particular view. Therefore, we need to look into the error message
        // field. The 'user unauthorized' means that the user is not logged in.
        if (err.status === 401 || (err.error && err.error.message === 'user unauthorized')) {
            this.serverData.invalidate()
            // User is apparently not logged in as it got Unauthorized error.
            // Remove the session information from the local storage and redirect
            // the user to the login page.
            this.auth.destroyLocalSession()
            // If there are multiple errors occuring one after another we don't
            // want to recursively append /login?returnUrl to the current route.
            if (this.router.url.lastIndexOf('/login', 0) !== 0) {
                this.router.navigate(['/login'], { queryParams: { returnUrl: this.router.url } })
            }
            return of(err.message)
        } else if (err.status === 403) {
            // User has no access to the given view. Let's redirect the
            // user to the error page.
            this.router.navigateByUrl('/forbidden', { skipLocationChange: true })
            return of(err.message)
        }
        return throwError(err)
    }

    intercept(req: HttpRequest<any>, next: HttpHandler): Observable<HttpEvent<any>> {
        return next.handle(req).pipe(catchError((x) => this.handleAuthError(x)))
    }
}
