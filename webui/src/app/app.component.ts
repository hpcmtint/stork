import { Component, OnInit } from '@angular/core'
import { Router, UrlSegment } from '@angular/router'
import { Observable } from 'rxjs'

import { MenuItem } from 'primeng/api'

import { AuthService } from './auth.service'
import { LoadingService } from './loading.service'

@Component({
    selector: 'app-root',
    templateUrl: './app.component.html',
    styleUrls: ['./app.component.sass'],
})
export class AppComponent implements OnInit {
    title = 'Stork'
    currentUser = null
    loadingInProgress = new Observable()

    menuItems: MenuItem[]

    constructor(private router: Router, private auth: AuthService, private loadingService: LoadingService) {
        this.auth.currentUser.subscribe(x => {
            this.currentUser = x
            this.initMenuItems()
        })
        this.loadingInProgress = this.loadingService.getState()
    }

    initMenuItems() {
        this.menuItems = []
        this.menuItems.push({
            label: 'Services',
            items: [
                {
                    label: 'Kea DHCP',
                    icon: 'fa fa-server',
                    routerLink: '/apps/kea/all',
                },
                // TODO: add support for BIND apps
                // {
                //     label: 'BIND DNS',
                //     icon: 'fa fa-server',
                //     routerLink: '/apps/bind/all',
                // },
                {
                    label: 'Machines',
                    icon: 'fa fa-server',
                    routerLink: '/machines/all',
                },
            ],
        })
        if (this.auth.superAdmin()) {
            this.menuItems = this.menuItems.concat([
            {
                label: 'Configuration',
                items: [
                    {
                        label: 'Users',
                        icon: 'fa fa-user',
                        routerLink: '/users',
                    },
                ],
            }])
        }
        this.menuItems.push(
            {
                label: 'Profile',
                items: [
                    {
                        label: 'Settings',
                        icon: 'fa fa-cog',
                        routerLink: '/settings',
                    },
                    {
                        label: 'Logout',
                        icon: 'pi pi-sign-out',
                        routerLink: '/logout',
                    },
                ],
            }
        )

    }

    ngOnInit() {
        this.initMenuItems()
    }

    signOut() {
        this.router.navigate(['/logout'])
    }
}
